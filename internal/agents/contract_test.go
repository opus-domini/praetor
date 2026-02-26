package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type scriptedCommandRunner struct {
	mu      sync.Mutex
	results []CommandResult
	idx     int
}

func (r *scriptedCommandRunner) Run(_ context.Context, _ CommandSpec) (CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.idx >= len(r.results) {
		return CommandResult{}, fmt.Errorf("unexpected command invocation %d", r.idx+1)
	}
	res := r.results[r.idx]
	r.idx++
	return res, nil
}

func TestAgentContractCLIAndREST(t *testing.T) {
	t.Parallel()

	cliRunner := &scriptedCommandRunner{results: []CommandResult{
		{
			Stdout:   `{"title":"generated","tasks":[{"id":"TASK-001","title":"task","executor":"codex","reviewer":"claude"}]}`,
			ExitCode: 0,
			Strategy: "process",
		},
		{
			Stdout:   "RESULT: PASS\nSUMMARY: done",
			ExitCode: 0,
			Strategy: "process",
		},
		{
			Stdout:   "DECISION: PASS",
			ExitCode: 0,
			Strategy: "process",
		},
	}}
	codex := NewCodexCLI("codex", cliRunner)
	runAgentContract(t, codex, "process")

	var mu sync.Mutex
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		call++
		current := call
		mu.Unlock()

		response := "RESULT: PASS\nSUMMARY: done"
		switch current {
		case 1:
			response = `{"title":"generated","tasks":[{"id":"TASK-001","title":"task","executor":"ollama","reviewer":"none"}]}`
		case 2:
			response = "RESULT: PASS\nSUMMARY: done"
		case 3:
			response = "DECISION: PASS"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"response": response})
	}))
	defer server.Close()

	ollama := NewOllamaREST(server.URL, "llama3", server.Client())
	runAgentContract(t, ollama, "structured")
}

func runAgentContract(t *testing.T, agent Agent, expectedStrategy string) {
	t.Helper()

	ctx := context.Background()
	planResp, err := agent.Plan(ctx, PlanRequest{
		Objective:        "improve pipeline",
		WorkspaceContext: "workspace",
		Workdir:          ".",
		OutputPrefix:     "planner",
		TaskLabel:        "planner",
	})
	if err != nil {
		t.Fatalf("plan failed for %s: %v", agent.ID(), err)
	}
	if strings.TrimSpace(planResp.Output) == "" {
		t.Fatalf("empty plan output for %s", agent.ID())
	}
	if got := strings.TrimSpace(planResp.Strategy); got != expectedStrategy {
		t.Fatalf("unexpected plan strategy for %s: got=%q want=%q", agent.ID(), got, expectedStrategy)
	}

	execResp, err := agent.Execute(ctx, ExecuteRequest{
		Prompt:       "do task",
		SystemPrompt: "system",
		Workdir:      ".",
		OutputPrefix: "executor",
		TaskLabel:    "TASK-001",
	})
	if err != nil {
		t.Fatalf("execute failed for %s: %v", agent.ID(), err)
	}
	if strings.TrimSpace(execResp.Output) == "" {
		t.Fatalf("empty execute output for %s", agent.ID())
	}
	if got := strings.TrimSpace(execResp.Strategy); got != expectedStrategy {
		t.Fatalf("unexpected execute strategy for %s: got=%q want=%q", agent.ID(), got, expectedStrategy)
	}

	reviewResp, err := agent.Review(ctx, ReviewRequest{
		Prompt:       "review",
		SystemPrompt: "system",
		Workdir:      ".",
		OutputPrefix: "reviewer",
		TaskLabel:    "TASK-001",
	})
	if err != nil {
		t.Fatalf("review failed for %s: %v", agent.ID(), err)
	}
	if reviewResp.Decision != DecisionPass {
		t.Fatalf("expected pass decision for %s, got %s", agent.ID(), reviewResp.Decision)
	}
	if got := strings.TrimSpace(reviewResp.Strategy); got != expectedStrategy {
		t.Fatalf("unexpected review strategy for %s: got=%q want=%q", agent.ID(), got, expectedStrategy)
	}
}
