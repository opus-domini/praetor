package orchestrator

import (
	"context"

	"github.com/opus-domini/praetor/internal/providers"
)

// ProviderID identifies an AI provider implementation.
type ProviderID string

const (
	ProviderClaude ProviderID = ProviderID(providers.Claude)
	ProviderCodex  ProviderID = ProviderID(providers.Codex)
)

// Request is the canonical execution input for a provider.
type Request struct {
	Provider ProviderID
	Prompt   string
}

// Result is the canonical provider response.
type Result struct {
	Provider ProviderID
	Response string
}

// Provider executes one orchestration request.
type Provider interface {
	ID() ProviderID
	Run(ctx context.Context, req Request) (Result, error)
}
