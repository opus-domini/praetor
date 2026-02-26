package codex

import (
	"context"
	"errors"
	"strings"

	"github.com/opus-domini/praetor/internal/providers"
)

// Provider adapts the Codex SDK to the providers interface.
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
func (p *Provider) ID() providers.ID {
	return providers.Codex
}

// Run executes a single turn through Codex.
func (p *Provider) Run(ctx context.Context, req providers.Request) (providers.Result, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return providers.Result{}, errors.New("prompt is required")
	}

	thread := p.client.StartThread(nil)
	turn, err := thread.Run(ctx, prompt, nil)
	if err != nil {
		return providers.Result{}, err
	}

	return providers.Result{
		Provider: p.ID(),
		Response: strings.TrimSpace(turn.FinalResponse),
	}, nil
}
