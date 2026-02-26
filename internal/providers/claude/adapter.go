package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/opus-domini/praetor/internal/providers"
)

// Provider adapts the Claude SDK to the providers interface.
type Provider struct {
	options Options
}

// NewProvider creates a claude-backed orchestration provider.
func NewProvider(options Options) *Provider {
	return &Provider{options: options}
}

// ID returns the provider identifier.
func (p *Provider) ID() providers.ID {
	return providers.Claude
}

// Run executes a one-shot prompt through Claude.
func (p *Provider) Run(ctx context.Context, req providers.Request) (providers.Result, error) {
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return providers.Result{}, errors.New("prompt is required")
	}

	msg, err := Prompt(ctx, prompt, p.options)
	if err != nil {
		return providers.Result{}, err
	}

	response, err := decodePromptResult(msg)
	if err != nil {
		return providers.Result{}, err
	}

	return providers.Result{
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
