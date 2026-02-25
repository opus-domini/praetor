package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ListSessionsOptions configures local session discovery.
type ListSessionsOptions struct {
	// Dir filters sessions for a specific project directory.
	// When set, this also includes known git worktrees for that directory.
	Dir string
	// Limit caps the number of sessions returned.
	// When <= 0, all sessions are returned.
	Limit int
	// ProjectsDir overrides the default ~/.claude/projects root.
	// Useful for tests or custom storage layouts.
	ProjectsDir string
}

// SDKSessionInfo mirrors the TypeScript SDK session metadata.
type SDKSessionInfo struct {
	SessionID    string `json:"sessionId"`
	Summary      string `json:"summary"`
	LastModified int64  `json:"lastModified"`
	FileSize     int64  `json:"fileSize"`
	CustomTitle  string `json:"customTitle,omitempty"`
	FirstPrompt  string `json:"firstPrompt,omitempty"`
	GitBranch    string `json:"gitBranch,omitempty"`
	CWD          string `json:"cwd,omitempty"`
}

type sessionMetadata struct {
	firstPrompt string
	summary     string
	cwd         string
	gitBranch   string
}

// ListSessions scans Claude local session files and returns metadata.
func ListSessions(options *ListSessionsOptions) ([]SDKSessionInfo, error) {
	var opts ListSessionsOptions
	if options != nil {
		opts = *options
	}

	projectsDir, err := resolveProjectsDir(opts.ProjectsDir)
	if err != nil {
		return nil, err
	}

	projectRoots, err := resolveProjectRoots(projectsDir, opts.Dir)
	if err != nil {
		return nil, err
	}
	if len(projectRoots) == 0 {
		return []SDKSessionInfo{}, nil
	}

	sessions := make([]SDKSessionInfo, 0, 64)
	for _, root := range projectRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			sessionPath := filepath.Join(root, entry.Name())
			stat, err := os.Stat(sessionPath)
			if err != nil {
				continue
			}

			meta, err := readSessionMetadata(sessionPath)
			if err != nil {
				continue
			}

			summary := strings.TrimSpace(meta.summary)
			if summary == "" {
				summary = summarizePrompt(meta.firstPrompt)
			}
			if summary == "" {
				summary = "Untitled session"
			}

			sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
			sessions = append(sessions, SDKSessionInfo{
				SessionID:    sessionID,
				Summary:      summary,
				LastModified: stat.ModTime().UnixMilli(),
				FileSize:     stat.Size(),
				FirstPrompt:  strings.TrimSpace(meta.firstPrompt),
				GitBranch:    strings.TrimSpace(meta.gitBranch),
				CWD:          strings.TrimSpace(meta.cwd),
			})
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].LastModified == sessions[j].LastModified {
			return sessions[i].SessionID > sessions[j].SessionID
		}
		return sessions[i].LastModified > sessions[j].LastModified
	})

	if opts.Limit > 0 && len(sessions) > opts.Limit {
		sessions = sessions[:opts.Limit]
	}
	return sessions, nil
}

func resolveProjectsDir(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

func resolveProjectRoots(projectsDir, dirFilter string) ([]string, error) {
	if strings.TrimSpace(dirFilter) == "" {
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			if os.IsNotExist(err) {
				return []string{}, nil
			}
			return nil, fmt.Errorf("read projects dir: %w", err)
		}
		roots := make([]string, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			roots = append(roots, filepath.Join(projectsDir, entry.Name()))
		}
		sort.Strings(roots)
		return roots, nil
	}

	filterDir, err := filepath.Abs(dirFilter)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute path for dir filter: %w", err)
	}
	filterDir = filepath.Clean(filterDir)

	projectNames := map[string]struct{}{
		encodeProjectDir(filterDir): {},
	}
	for _, wt := range gitWorktreePaths(filterDir) {
		projectNames[encodeProjectDir(wt)] = struct{}{}
	}

	roots := make([]string, 0, len(projectNames))
	for name := range projectNames {
		path := filepath.Join(projectsDir, name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		roots = append(roots, path)
	}
	sort.Strings(roots)
	return roots, nil
}

func encodeProjectDir(dir string) string {
	clean := filepath.Clean(dir)
	clean = strings.ReplaceAll(clean, "\\", "/")
	clean = strings.ReplaceAll(clean, "/", "-")
	return clean
}

func gitWorktreePaths(dir string) []string {
	cmd := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	paths := make([]string, 0, 4)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		paths = append(paths, filepath.Clean(abs))
	}
	return paths
}

func readSessionMetadata(path string) (sessionMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionMetadata{}, fmt.Errorf("open session file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	meta := sessionMetadata{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var payload struct {
			Type      string `json:"type"`
			Summary   string `json:"summary"`
			CWD       string `json:"cwd"`
			GitBranch string `json:"gitBranch"`
			Message   struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			continue
		}

		if payload.CWD != "" {
			meta.cwd = payload.CWD
		}
		if payload.GitBranch != "" {
			meta.gitBranch = payload.GitBranch
		}
		if payload.Type == "summary" && strings.TrimSpace(payload.Summary) != "" {
			meta.summary = strings.TrimSpace(payload.Summary)
		}
		if meta.firstPrompt == "" && payload.Type == "user" && payload.Message.Role == "user" {
			text := extractFirstUserText(payload.Message.Content)
			if text != "" {
				meta.firstPrompt = text
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return sessionMetadata{}, fmt.Errorf("scan session file: %w", err)
	}
	return meta, nil
}

func extractFirstUserText(content any) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		for _, item := range value {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := obj["type"].(string)
			if typ != "text" {
				continue
			}
			text, _ := obj["text"].(string)
			text = strings.TrimSpace(text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func summarizePrompt(prompt string) string {
	s := strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if s == "" {
		return ""
	}
	const maxRunes = 120
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes-1]) + "..."
}
