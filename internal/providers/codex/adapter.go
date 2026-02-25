package codex

import (
	"context"
	"errors"
	"strings"

	"github.com/opus-domini/praetor/internal/orchestrator"
)

// Provider adapts the Codex SDK to the orchestrator interface.
type Provider struct {
	client *Codex
}

// NewProvider creates a codex-backed orchestration provider.
func NewProvider(options CodexOptions) (*Provider, error) {
	client, err := New(options)
	if err != nil {
		return nil, err
	}
	return &Provider{client: client}, nil
}

// ID returns the provider identifier.
func (p *Provider) ID() orchestrator.ProviderID {
	return orchestrator.ProviderCodex
}

// Run executes a single turn through Codex.
func (p *Provider) Run(ctx context.Context, req orchestrator.Request) (orchestrator.Result, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return orchestrator.Result{}, errors.New("prompt is required")
	}

	thread := p.client.StartThread(nil)
	turn, err := thread.Run(ctx, prompt, nil)
	if err != nil {
		return orchestrator.Result{}, err
	}

	return orchestrator.Result{
		Provider: p.ID(),
		Response: strings.TrimSpace(turn.FinalResponse),
	}, nil
}
