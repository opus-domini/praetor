package orchestrator

import "context"

// ProviderID identifies an AI provider implementation.
type ProviderID string

const (
	ProviderClaude ProviderID = "claude"
	ProviderCodex  ProviderID = "codex"
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
