package state

import (
	"strings"

	"github.com/opus-domini/praetor/internal/workspace"
)

// ResolveProjectHome returns the project home from explicit input or derived project scope.
func ResolveProjectHome(explicitRoot, projectDir string) (string, error) {
	explicitRoot = strings.TrimSpace(explicitRoot)
	if explicitRoot != "" {
		return workspace.ExpandPath(explicitRoot)
	}
	return ProjectHomeForDir(projectDir)
}

// ProjectHomeForDir resolves the per-project home directory.
func ProjectHomeForDir(projectDir string) (string, error) {
	projectRoot, err := workspace.ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return DefaultProjectRoot(projectRoot)
}

// ProjectRuntimeKeyForDir resolves the stable project key used for runtime artifacts.
func ProjectRuntimeKeyForDir(projectDir string) (string, error) {
	projectRoot, err := workspace.ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return ProjectRuntimeKey(projectRoot), nil
}
