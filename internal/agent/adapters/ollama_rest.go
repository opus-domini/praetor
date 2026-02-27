package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
)

// OllamaREST is a REST-backed Agent implementation for local/remote Ollama.
type OllamaREST struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOllamaREST(baseURL, model string, client *http.Client) *OllamaREST {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "llama3"
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &OllamaREST{BaseURL: strings.TrimRight(baseURL, "/"), Model: model, Client: client}
}

func (a *OllamaREST) ID() agent.ID { return agent.Ollama }

func (a *OllamaREST) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportREST,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: false,
	}
}

func (a *OllamaREST) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
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
	obj, err := ExtractJSONObject(resp.Output)
	if err == nil {
		resp.Manifest = json.RawMessage(obj)
	}
	return resp, nil
}

func (a *OllamaREST) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	resp, err := a.generate(ctx, req.Model, ComposePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return agent.ExecuteResponse{}, err
	}
	return agent.ExecuteResponse{Output: resp.Output, Model: resp.Model, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *OllamaREST) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.generate(ctx, req.Model, ComposePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return agent.ReviewResponse{}, err
	}
	decision, reason := ParseReview(resp.Output)
	return agent.ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *OllamaREST) generate(ctx context.Context, model, prompt string) (agent.PlanResponse, error) {
	start := time.Now()
	if strings.TrimSpace(prompt) == "" {
		return agent.PlanResponse{}, errors.New("prompt is required")
	}
	if strings.TrimSpace(model) == "" {
		model = a.Model
	}

	payload := map[string]any{
		"model":  strings.TrimSpace(model),
		"prompt": strings.TrimSpace(prompt),
		"stream": false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("encode ollama request: %w", err)
	}

	url := a.BaseURL + "/api/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("build ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := a.Client.Do(httpReq)
	if err != nil {
		return agent.PlanResponse{}, fmt.Errorf("call ollama: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return agent.PlanResponse{}, fmt.Errorf("ollama returned status %s", httpResp.Status)
	}

	decoded := struct {
		Response string `json:"response"`
	}{
		Response: "",
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return agent.PlanResponse{}, fmt.Errorf("decode ollama response: %w", err)
	}

	return agent.PlanResponse{
		Output:    strings.TrimSpace(decoded.Response),
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  "structured",
	}, nil
}
