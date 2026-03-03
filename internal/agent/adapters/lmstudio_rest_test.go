package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
)

func TestLMStudioExecute(t *testing.T) {
	t.Setenv("LMSTUDIO_API_KEY", "test-token")
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		response := map[string]any{
			"model": "local-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "RESULT: PASS"}},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewLMStudioREST(server.URL, "local-model", "LMSTUDIO_API_KEY", server.Client())
	resp, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "Validate this",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if authHeader != "Bearer test-token" {
		t.Fatalf("unexpected auth header: %q", authHeader)
	}
	if strings.TrimSpace(resp.Output) != "RESULT: PASS" {
		t.Fatalf("unexpected response output: %q", resp.Output)
	}
	if resp.Strategy != "structured" {
		t.Fatalf("unexpected strategy: %q", resp.Strategy)
	}
}

func TestLMStudioExecuteWithoutAPIKey(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		response := map[string]any{
			"model": "local-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "hello"}},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewLMStudioREST(server.URL, "local-model", "LMSTUDIO_API_KEY", server.Client())
	resp, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "Hello",
		Workdir: ".",
	})
	if err != nil {
		t.Fatalf("expected no error without API key, got: %v", err)
	}
	if authHeader != "" {
		t.Fatalf("expected no auth header when env var is empty, got: %q", authHeader)
	}
	if strings.TrimSpace(resp.Output) != "hello" {
		t.Fatalf("unexpected response output: %q", resp.Output)
	}
}

func TestLMStudioPlanExtractsJSON(t *testing.T) {
	t.Parallel()

	planJSON := `{"name":"test","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"test","acceptance":["ok"]}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		response := map[string]any{
			"model": "local-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "Here is the plan:\n" + planJSON}},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewLMStudioREST(server.URL, "local-model", "", server.Client())
	resp, err := provider.Plan(context.Background(), agent.PlanRequest{
		Objective: "Build a feature",
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(resp.Manifest) == 0 {
		t.Fatal("expected non-empty manifest")
	}
}

func TestLMStudioReviewParsesDecision(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		response := map[string]any{
			"model": "local-model",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "DECISION: PASS\nREASON: looks good"}},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewLMStudioREST(server.URL, "local-model", "", server.Client())
	resp, err := provider.Review(context.Background(), agent.ReviewRequest{
		Prompt: "Review this",
	})
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if resp.Decision != agent.DecisionPass {
		t.Fatalf("expected PASS, got %q", resp.Decision)
	}
}
