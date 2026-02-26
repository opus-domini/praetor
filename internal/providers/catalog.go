// Package providers is deprecated.
// Use internal/agents as the canonical provider abstraction layer.
package providers

import (
	"context"
	"strings"
)

// ID identifies one supported provider implementation.
type ID string

const (
	Codex  ID = "codex"
	Claude ID = "claude"
	Gemini ID = "gemini"
	Ollama ID = "ollama"
)

// Normalize canonicalizes provider identifiers for validation and routing.
func Normalize(raw string) ID {
	return ID(strings.ToLower(strings.TrimSpace(raw)))
}

// IsSupported reports whether id maps to a built-in provider.
func IsSupported(id ID) bool {
	switch id {
	case Codex, Claude, Gemini, Ollama:
		return true
	default:
		return false
	}
}

// Request is the canonical execution input for a provider.
type Request struct {
	Provider ID
	Prompt   string
}

// Result is the canonical provider response.
type Result struct {
	Provider ID
	Response string
}

// Provider executes one orchestration request.
type Provider interface {
	ID() ID
	Run(ctx context.Context, req Request) (Result, error)
}
