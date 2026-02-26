package loop

import (
	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/opus-domini/praetor/internal/workspace"
)

// ResolveStateRoot returns the state root from explicit input or derived project scope.
func ResolveStateRoot(explicitRoot, projectDir string) (string, error) {
	return localstate.ResolveStateRoot(explicitRoot, projectDir)
}

// ProjectStateRootForDir resolves the per-project state root using XDG paths
// with read-fallback to legacy ~/.praetor/projects.
func ProjectStateRootForDir(projectDir string) (string, error) {
	return localstate.ProjectStateRootForDir(projectDir)
}

// ProjectCacheRootForDir resolves the per-project cache root using XDG paths.
func ProjectCacheRootForDir(projectDir string) (string, error) {
	return localstate.ProjectCacheRootForDir(projectDir)
}

// ResolveCacheRoot returns the cache root from explicit input or derived project scope.
func ResolveCacheRoot(explicitRoot, projectDir string) (string, error) {
	return localstate.ResolveCacheRoot(explicitRoot, projectDir)
}

// ProjectRuntimeKeyForDir resolves the stable project key used for runtime artifacts.
func ProjectRuntimeKeyForDir(projectDir string) (string, error) {
	return localstate.ProjectRuntimeKeyForDir(projectDir)
}

// ResolveProjectRoot resolves the git repository root for a directory.
func ResolveProjectRoot(dir string) (string, error) {
	return workspace.ResolveProjectRoot(dir)
}

func projectRuntimeKey(projectRoot string) string {
	return localstate.ProjectRuntimeKey(projectRoot)
}

const maxWorkspaceManifestSize = workspace.MaxManifestSize

// WorkspaceManifest holds one resolved workspace-level context file.
type WorkspaceManifest = workspace.Manifest

// ReadWorkspaceManifest reads local workspace directives from the project root.
// Priority order:
//  1. praetor.yaml
//  2. praetor.yml
//  3. praetor.md
//
// Returns zero value and nil error when no manifest file is present.
func ReadWorkspaceManifest(projectRoot string) (WorkspaceManifest, error) {
	return workspace.ReadManifest(projectRoot)
}

// ReadProjectContext reads a praetor.md file from the project root.
// Returns empty string and nil error if the file does not exist.
// Content is truncated at 16 KiB; the second return indicates truncation.
func ReadProjectContext(projectRoot string) (string, bool, error) {
	return workspace.ReadProjectContext(projectRoot)
}

func expandPath(path string) (string, error) {
	return workspace.ExpandPath(path)
}
