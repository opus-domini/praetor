package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaRESTExecute(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"RESULT: PASS\nSUMMARY: ok"}`))
	}))
	defer server.Close()

	agent := NewOllamaREST(server.URL, "llama3.1", server.Client())
	resp, err := agent.Execute(context.Background(), ExecuteRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.Output == "" {
		t.Fatal("expected output")
	}
}

func TestOllamaRESTPlanExtractsManifest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"{\"title\":\"p\",\"tasks\":[]}"}`))
	}))
	defer server.Close()

	agent := NewOllamaREST(server.URL, "llama3.1", server.Client())
	resp, err := agent.Plan(context.Background(), PlanRequest{Objective: "build"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(resp.Manifest) == 0 {
		t.Fatal("expected manifest")
	}
	payload := map[string]any{}
	if err := json.Unmarshal(resp.Manifest, &payload); err != nil {
		t.Fatalf("manifest is not valid json: %v", err)
	}
}
