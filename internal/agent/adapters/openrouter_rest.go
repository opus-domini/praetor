package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
	agenttext "github.com/opus-domini/praetor/internal/agent/text"
)

// OpenRouterREST is a REST-backed Agent implementation for OpenRouter.
type OpenRouterREST struct {
	BaseURL   string
	Model     string
	APIKeyEnv string
	Client    *http.Client
}

func NewOpenRouterREST(baseURL, model, apiKeyEnv string, client *http.Client) *OpenRouterREST {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENROUTER_API_KEY"
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &OpenRouterREST{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		Client:    client,
	}
}

func (a *OpenRouterREST) ID() agent.ID { return agent.OpenRouter }

func (a *OpenRouterREST) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportREST,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: true,
	}
}

func (a *OpenRouterREST) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return agent.PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY JSON with a dependency-aware execution plan for:\n\n" + objective
	if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	resp, err := a.generate(ctx, req.Model, prompt)
	if err != nil {
		return agent.PlanResponse{}, err
	}
	obj, err := agenttext.ExtractJSONObject(resp.Output)
	if err == nil {
		resp.Manifest = json.RawMessage(obj)
	}
	return resp, nil
}

func (a *OpenRouterREST) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	resp, err := a.generate(ctx, req.Model, agenttext.ComposePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return agent.ExecuteResponse{}, err
	}
	return agent.ExecuteResponse{Output: resp.Output, Model: resp.Model, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *OpenRouterREST) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.generate(ctx, req.Model, agenttext.ComposePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return agent.ReviewResponse{}, err
	}
	decision, reason := agenttext.ParseReview(resp.Output)
	return agent.ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *OpenRouterREST) generate(ctx context.Context, model, prompt string) (agent.PlanResponse, error) {
	start := time.Now()
	if strings.TrimSpace(prompt) == "" {
		return agent.PlanResponse{}, errors.New("prompt is required")
	}
	if strings.TrimSpace(model) == "" {
		model = a.Model
	}

	apiKey := strings.TrimSpace(os.Getenv(a.APIKeyEnv))
	if apiKey == "" {
		return agent.PlanResponse{}, fmt.Errorf("openrouter api key not found in %s", a.APIKeyEnv)
	}

	payload := map[string]any{
		"model": strings.TrimSpace(model),
		"messages": []map[string]string{
			{"role": "user", "content": strings.TrimSpace(prompt)},
		},
		"stream": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("encode openrouter request: %w", err)
	}

	url := a.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("build openrouter request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.Client.Do(httpReq)
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("call openrouter: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return agent.PlanResponse{}, fmt.Errorf("openrouter returned status %s", httpResp.Status)
	}

	decoded := struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}{
		Choices: nil,
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return agent.PlanResponse{}, fmt.Errorf("decode openrouter response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return agent.PlanResponse{}, errors.New("openrouter returned no choices")
	}
	resolvedModel := strings.TrimSpace(decoded.Model)
	if resolvedModel == "" {
		resolvedModel = strings.TrimSpace(model)
	}
	return agent.PlanResponse{
		Output:    strings.TrimSpace(decoded.Choices[0].Message.Content),
		Model:     resolvedModel,
		DurationS: time.Since(start).Seconds(),
		Strategy:  "structured",
	}, nil
}
