// Package app provides bootstrap and dependency wiring for the praetor CLI.
// It bridges domain, state, orchestration, and runtime packages without
// exposing internal construction details to the CLI layer.
package app

import (
	"github.com/opus-domini/praetor/internal/state"
	"github.com/opus-domini/praetor/internal/workspace"
)

// ResolveStateRoot returns the state root from explicit user input or
// derived from the project directory via XDG conventions.
func ResolveStateRoot(explicitRoot, projectDir string) (string, error) {
	return state.ResolveStateRoot(explicitRoot, projectDir)
}

// ResolveCacheRoot returns the cache root from explicit user input or
// derived from the project directory via XDG conventions.
func ResolveCacheRoot(explicitRoot, projectDir string) (string, error) {
	return state.ResolveCacheRoot(explicitRoot, projectDir)
}

// ResolveProjectRoot resolves the git repository root for a directory.
func ResolveProjectRoot(dir string) (string, error) {
	return workspace.ResolveProjectRoot(dir)
}
