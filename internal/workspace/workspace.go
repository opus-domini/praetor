package workspace

import (
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
	Path      string
	Context   string
	Truncated bool
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
	if strings.HasSuffix(strings.ToLower(path), ".yaml") || strings.HasSuffix(strings.ToLower(path), ".yml") {
		context = "```yaml\n" + context + "\n```"
	}
	return Manifest{
		Path:      path,
		Context:   context,
		Truncated: truncated,
	}, nil
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
