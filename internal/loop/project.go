package loop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/paths"
)

// ResolveStateRoot returns the state root from explicit input or derived project scope.
func ResolveStateRoot(explicitRoot, projectDir string) (string, error) {
	explicitRoot = strings.TrimSpace(explicitRoot)
	if explicitRoot != "" {
		return expandPath(explicitRoot)
	}
	return ProjectStateRootForDir(projectDir)
}

// ProjectStateRootForDir resolves the per-project state root using XDG paths
// with read-fallback to legacy ~/.praetor/projects.
func ProjectStateRootForDir(projectDir string) (string, error) {
	projectRoot, err := ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	xdgRoot, err := paths.DefaultProjectStateRoot(projectRoot)
	if err != nil {
		return "", err
	}
	// Read-fallback: if XDG state doesn't exist, check legacy location.
	if _, statErr := os.Stat(xdgRoot); statErr != nil {
		if legacy := paths.LegacyProjectStateRoot(projectRoot); legacy != "" {
			return legacy, nil
		}
	}
	return xdgRoot, nil
}

// ProjectCacheRootForDir resolves the per-project cache root using XDG paths.
func ProjectCacheRootForDir(projectDir string) (string, error) {
	projectRoot, err := ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return paths.DefaultProjectCacheRoot(projectRoot)
}

// ResolveCacheRoot returns the cache root from explicit input or derived project scope.
func ResolveCacheRoot(explicitRoot, projectDir string) (string, error) {
	explicitRoot = strings.TrimSpace(explicitRoot)
	if explicitRoot != "" {
		return expandPath(explicitRoot)
	}
	return ProjectCacheRootForDir(projectDir)
}

// ProjectRuntimeKeyForDir resolves the stable project key used for runtime artifacts.
func ProjectRuntimeKeyForDir(projectDir string) (string, error) {
	projectRoot, err := ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return paths.ProjectRuntimeKey(projectRoot), nil
}

// ResolveProjectRoot resolves the git repository root for a directory.
func ResolveProjectRoot(dir string) (string, error) {
	absDir, err := expandPath(dir)
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

func projectRuntimeKey(projectRoot string) string {
	return paths.ProjectRuntimeKey(projectRoot)
}

const maxProjectContextSize = 16 * 1024 // 16 KiB

// ReadProjectContext reads a praetor.md file from the project root.
// Returns empty string and nil error if the file does not exist.
// Content is truncated at 16 KiB; the second return indicates truncation.
func ReadProjectContext(projectRoot string) (string, bool, error) {
	path := filepath.Join(projectRoot, "praetor.md")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read project context: %w", err)
	}
	content := strings.TrimSpace(string(data))
	if len(content) > maxProjectContextSize {
		return content[:maxProjectContextSize], true, nil
	}
	return content, false, nil
}

func expandPath(path string) (string, error) {
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
