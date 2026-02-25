package loop

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// PraetorHomeDirName is the global home directory for Praetor runtime data.
	PraetorHomeDirName = ".praetor"
	// PraetorProjectsDirName holds per-project runtime directories.
	PraetorProjectsDirName = "projects"
)

// ResolveStateRoot returns the state root from explicit input or derived project scope.
func ResolveStateRoot(explicitRoot, projectDir string) (string, error) {
	explicitRoot = strings.TrimSpace(explicitRoot)
	if explicitRoot != "" {
		return expandPath(explicitRoot)
	}
	return ProjectStateRootForDir(projectDir)
}

// ProjectStateRootForDir resolves the per-project state root under ~/.praetor/projects.
func ProjectStateRootForDir(projectDir string) (string, error) {
	projectRoot, err := ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	key := projectRuntimeKey(projectRoot)
	return filepath.Join(homeDir, PraetorHomeDirName, PraetorProjectsDirName, key), nil
}

// ProjectRuntimeKeyForDir resolves the stable project key used for runtime artifacts.
func ProjectRuntimeKeyForDir(projectDir string) (string, error) {
	projectRoot, err := ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return projectRuntimeKey(projectRoot), nil
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
	baseName := strings.TrimSpace(filepath.Base(projectRoot))
	baseName = strings.Trim(baseName, ".")
	if baseName == "" {
		baseName = "project"
	}

	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-", "\n", "-", "\r", "-")
	baseName = replacer.Replace(baseName)

	hash := sha1.Sum([]byte(projectRoot))
	hashPart := hex.EncodeToString(hash[:])[:12]
	return baseName + "-" + hashPart
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
