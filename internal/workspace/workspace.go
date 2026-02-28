package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const MaxManifestSize = 16 * 1024 // 16 KiB

// Manifest holds one resolved workspace-level context file.
type Manifest struct {
	Path       string
	RawContext string
	Context    string
	Truncated  bool
	Hash       string
}

// ResolveProjectRoot resolves the git repository root for a directory.
func ResolveProjectRoot(dir string) (string, error) {
	absDir, err := ExpandPath(dir)
	if err != nil {
		return "", err
	}
	if absDir == "" {
		absDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}

	cmd := exec.Command("git", "-C", absDir, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr == "" {
				stderr = "not a git repository"
			}
			return "", fmt.Errorf("resolve git project root from %s: %s", absDir, stderr)
		}
		return "", fmt.Errorf("resolve git project root from %s: %w", absDir, err)
	}

	projectRoot := strings.TrimSpace(string(output))
	if projectRoot == "" {
		return "", fmt.Errorf("resolve git project root from %s: empty result", absDir)
	}

	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("normalize git project root: %w", err)
	}
	if realPath, err := filepath.EvalSymlinks(projectRoot); err == nil {
		projectRoot = realPath
	}
	return projectRoot, nil
}

// ReadManifest reads local workspace directives from the project root.
// Priority order:
//  1. praetor.yaml
//  2. praetor.yml
//  3. praetor.md
//
// Returns zero value and nil error when no manifest file is present.
func ReadManifest(projectRoot string) (Manifest, error) {
	candidates := []string{
		filepath.Join(projectRoot, "praetor.yaml"),
		filepath.Join(projectRoot, "praetor.yml"),
		filepath.Join(projectRoot, "praetor.md"),
	}
	for _, candidate := range candidates {
		manifest, err := readManifestFile(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Manifest{}, err
		}
		return manifest, nil
	}
	return Manifest{}, nil
}

// ReadProjectContext reads a praetor.md file from the project root.
// Returns empty string and nil error if the file does not exist.
// Content is truncated at MaxManifestSize; the second return indicates truncation.
func ReadProjectContext(projectRoot string) (string, bool, error) {
	manifest, err := readManifestFile(filepath.Join(projectRoot, "praetor.md"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read project context: %w", err)
	}
	return manifest.Context, manifest.Truncated, nil
}

func readManifestFile(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	context := strings.TrimSpace(string(data))
	truncated := false
	if len(context) > MaxManifestSize {
		context = context[:MaxManifestSize]
		truncated = true
	}
	rawContext := context
	if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
		if normalized := normalizeWorkspaceYAML(context); normalized != "" {
			context = normalized
		} else {
			context = "```yaml\n" + context + "\n```"
		}
	}
	return Manifest{
		Path:       path,
		RawContext: rawContext,
		Context:    context,
		Truncated:  truncated,
		Hash:       sha256Hex(data),
	}, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type parsedWorkspaceManifest struct {
	Version      string
	Instructions []string
	Constraints  []string
	TestCommands []string
}

func normalizeWorkspaceYAML(input string) string {
	parsed, ok := parseWorkspaceYAML(input)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Workspace Manifest")
	if strings.TrimSpace(parsed.Version) != "" {
		b.WriteString("\n")
		b.WriteString("Version: ")
		b.WriteString(strings.TrimSpace(parsed.Version))
	}
	if len(parsed.Instructions) > 0 {
		b.WriteString("\n\nInstructions:")
		for _, v := range parsed.Instructions {
			b.WriteString("\n- ")
			b.WriteString(v)
		}
	}
	if len(parsed.Constraints) > 0 {
		b.WriteString("\n\nConstraints:")
		for _, v := range parsed.Constraints {
			b.WriteString("\n- ")
			b.WriteString(v)
		}
	}
	if len(parsed.TestCommands) > 0 {
		b.WriteString("\n\nTest Commands:")
		for _, v := range parsed.TestCommands {
			b.WriteString("\n- ")
			b.WriteString(v)
		}
	}
	return strings.TrimSpace(b.String())
}

func parseWorkspaceYAML(input string) (parsedWorkspaceManifest, bool) {
	lines := strings.Split(strings.TrimSpace(input), "\n")
	if len(lines) == 0 {
		return parsedWorkspaceManifest{}, false
	}
	out := parsedWorkspaceManifest{}
	currentList := ""
	recognized := false

	appendItem := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		switch currentList {
		case "instructions":
			out.Instructions = append(out.Instructions, value)
		case "constraints":
			out.Constraints = append(out.Constraints, value)
		case "test_commands":
			out.TestCommands = append(out.TestCommands, value)
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			appendItem(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			// Not a subset we can normalize.
			return parsedWorkspaceManifest{}, false
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)
		switch key {
		case "version":
			recognized = true
			currentList = ""
			out.Version = strings.Trim(value, "\"'")
		case "instructions", "constraints", "test_commands":
			recognized = true
			currentList = key
			if value != "" {
				appendItem(strings.Trim(value, "\"'"))
			}
		default:
			// Unknown key: fallback to raw fenced YAML.
			return parsedWorkspaceManifest{}, false
		}
	}
	if !recognized {
		return parsedWorkspaceManifest{}, false
	}
	return out, true
}

func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}

	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		if path == "~" {
			path = homeDir
		} else if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
			path = filepath.Join(homeDir, path[2:])
		}
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path %s: %w", path, err)
	}
	return absPath, nil
}
