package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Engine dispatches requests to registered providers.
type Engine struct {
	registry *Registry
}

// New creates an orchestration engine.
func New(registry *Registry) *Engine {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Engine{registry: registry}
}

// Run dispatches one request to the target provider.
func (e *Engine) Run(ctx context.Context, req Request) (Result, error) {
	providerID := ProviderID(strings.TrimSpace(string(req.Provider)))
	if providerID == "" {
		return Result{}, errors.New("provider is required")
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return Result{}, errors.New("prompt is required")
	}

	provider, ok := e.registry.Get(providerID)
	if !ok {
		return Result{}, fmt.Errorf("unknown provider %q", providerID)
	}

	result, err := provider.Run(ctx, Request{
		Provider: providerID,
		Prompt:   prompt,
	})
	if err != nil {
		return Result{}, err
	}
	if result.Provider == "" {
		result.Provider = providerID
	}
	return result, nil
}

// Providers returns registered provider IDs.
func (e *Engine) Providers() []ProviderID {
	return e.registry.IDs()
}
