package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/opus-domini/praetor/internal/orchestrator"
)

// Provider adapts the Claude SDK to the orchestrator interface.
type Provider struct {
	options Options
}

// NewProvider creates a claude-backed orchestration provider.
func NewProvider(options Options) *Provider {
	return &Provider{options: options}
}

// ID returns the provider identifier.
func (p *Provider) ID() orchestrator.ProviderID {
	return orchestrator.ProviderClaude
}

// Run executes a one-shot prompt through Claude.
func (p *Provider) Run(ctx context.Context, req orchestrator.Request) (orchestrator.Result, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return orchestrator.Result{}, errors.New("prompt is required")
	}

	msg, err := Prompt(ctx, prompt, p.options)
	if err != nil {
		return orchestrator.Result{}, err
	}

	response, err := decodePromptResult(msg)
	if err != nil {
		return orchestrator.Result{}, err
	}

	return orchestrator.Result{
		Provider: p.ID(),
		Response: response,
	}, nil
}

func decodePromptResult(msg SDKMessage) (string, error) {
	result := ResultMessage{}
	if err := json.Unmarshal(msg.Raw, &result); err != nil {
		return "", err
	}

	if result.IsError {
		errMessage := strings.TrimSpace(result.Result)
		if errMessage == "" {
			errMessage = "claude returned an error result"
		}
		return "", errors.New(errMessage)
	}

	if resultText := strings.TrimSpace(result.Result); resultText != "" {
		return resultText, nil
	}
	return strings.TrimSpace(string(msg.Raw)), nil
}
