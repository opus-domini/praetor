package state

import (
	"os"
	"strings"

	"github.com/opus-domini/praetor/internal/paths"
	"github.com/opus-domini/praetor/internal/workspace"
)

// ResolveStateRoot returns the state root from explicit input or derived project scope.
func ResolveStateRoot(explicitRoot, projectDir string) (string, error) {
	explicitRoot = strings.TrimSpace(explicitRoot)
	if explicitRoot != "" {
		return workspace.ExpandPath(explicitRoot)
	}
	return ProjectStateRootForDir(projectDir)
}

// ProjectStateRootForDir resolves the per-project state root using XDG paths
// with read-fallback to legacy ~/.praetor/projects.
func ProjectStateRootForDir(projectDir string) (string, error) {
	projectRoot, err := workspace.ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	xdgRoot, err := paths.DefaultProjectStateRoot(projectRoot)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(xdgRoot); statErr != nil {
		if legacy := paths.LegacyProjectStateRoot(projectRoot); legacy != "" {
			return legacy, nil
		}
	}
	return xdgRoot, nil
}

// ProjectCacheRootForDir resolves the per-project cache root using XDG paths.
func ProjectCacheRootForDir(projectDir string) (string, error) {
	projectRoot, err := workspace.ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return paths.DefaultProjectCacheRoot(projectRoot)
}

// ResolveCacheRoot returns the cache root from explicit input or derived project scope.
func ResolveCacheRoot(explicitRoot, projectDir string) (string, error) {
	explicitRoot = strings.TrimSpace(explicitRoot)
	if explicitRoot != "" {
		return workspace.ExpandPath(explicitRoot)
	}
	return ProjectCacheRootForDir(projectDir)
}

// ProjectRuntimeKeyForDir resolves the stable project key used for runtime artifacts.
func ProjectRuntimeKeyForDir(projectDir string) (string, error) {
	projectRoot, err := workspace.ResolveProjectRoot(projectDir)
	if err != nil {
		return "", err
	}
	return paths.ProjectRuntimeKey(projectRoot), nil
}
