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

func TestOpenRouterExecute(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-token")
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		response := map[string]any{
			"model": "openai/gpt-4o-mini",
			"choices": []map[string]any{
				{"message": map[string]string{"content": "RESULT: PASS"}},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenRouterREST(server.URL, "openai/gpt-4o-mini", "OPENROUTER_API_KEY", server.Client())
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

func TestOpenRouterRequiresAPIKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	provider := NewOpenRouterREST(server.URL, "openai/gpt-4o-mini", "OPENROUTER_API_KEY", server.Client())
	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{Prompt: "hello"})
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}
