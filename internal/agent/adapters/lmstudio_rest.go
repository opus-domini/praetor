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
)

// LMStudioREST is a REST-backed Agent implementation for LM Studio.
// It uses the OpenAI-compatible API with optional authentication.
type LMStudioREST struct {
	BaseURL   string
	Model     string
	APIKeyEnv string
	Client    *http.Client
}

func NewLMStudioREST(baseURL, model, apiKeyEnv string, client *http.Client) *LMStudioREST {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "http://localhost:1234"
	}
	model = strings.TrimSpace(model)
	apiKeyEnv = strings.TrimSpace(apiKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = "LMSTUDIO_API_KEY"
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &LMStudioREST{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Model:     model,
		APIKeyEnv: apiKeyEnv,
		Client:    client,
	}
}

func (a *LMStudioREST) ID() agent.ID { return agent.LMStudio }

func (a *LMStudioREST) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportREST,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: true,
	}
}

func (a *LMStudioREST) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return agent.PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY JSON with a dependency-aware execution plan for:\n\n" + objective
	if req.PromptEngine != nil {
		if s, err := req.PromptEngine.Render("adapter.plan", adapterPlanData(objective, req.WorkspaceContext)); err == nil {
			prompt = s
		}
	} else if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	resp, err := a.generate(ctx, req.Model, prompt)
	if err != nil {
		return agent.PlanResponse{}, err
	}
	obj, err := ExtractJSONObject(resp.Output)
	if err == nil {
		resp.Manifest = json.RawMessage(obj)
	}
	return resp, nil
}

func (a *LMStudioREST) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	resp, err := a.generate(ctx, req.Model, ComposePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return agent.ExecuteResponse{}, err
	}
	return agent.ExecuteResponse{Output: resp.Output, Model: resp.Model, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *LMStudioREST) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.generate(ctx, req.Model, ComposePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return agent.ReviewResponse{}, err
	}
	decision, reason := ParseReview(resp.Output)
	return agent.ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *LMStudioREST) generate(ctx context.Context, model, prompt string) (agent.PlanResponse, error) {
	start := time.Now()
	if strings.TrimSpace(prompt) == "" {
		return agent.PlanResponse{}, errors.New("prompt is required")
	}
	if strings.TrimSpace(model) == "" {
		model = a.Model
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
		return agent.PlanResponse{}, fmt.Errorf("encode lmstudio request: %w", err)
	}

	url := a.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("build lmstudio request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Auth is optional — only set Authorization header when the env var is non-empty.
	if apiKey := strings.TrimSpace(os.Getenv(a.APIKeyEnv)); apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	httpResp, err := a.Client.Do(httpReq)
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("call lmstudio: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return agent.PlanResponse{}, fmt.Errorf("lmstudio returned status %s", httpResp.Status)
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
		return agent.PlanResponse{}, fmt.Errorf("decode lmstudio response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return agent.PlanResponse{}, errors.New("lmstudio returned no choices")
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
