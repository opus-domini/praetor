package adapters

import (
	"context"
	"os"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type codexPlanCaptureRunner struct {
	spec   runner.CommandSpec
	result runner.CommandResult
}

func (r *codexPlanCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.spec = spec
	for i := range spec.Args[:len(spec.Args)-1] {
		if spec.Args[i] == "--output-last-message" {
			plan := `{"name":"test-plan","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}`
			if err := os.WriteFile(spec.Args[i+1], []byte(plan), 0o644); err != nil {
				return runner.CommandResult{}, err
			}
			break
		}
	}
	return r.result, nil
}

func TestCodexPlanUsesOutputSchemaAndReadOnlySandbox(t *testing.T) {
	t.Parallel()

	commandRunner := &codexPlanCaptureRunner{result: runner.CommandResult{
		Stdout: `{"type":"thread.started","thread_id":"abc"}
{"type":"turn.completed"}`,
	}}
	provider := NewCodexCLI("codex", commandRunner)

	resp, err := provider.Plan(context.Background(), agent.PlanRequest{
		Objective: "Plan the migration",
		Workdir:   ".",
	})
	if err != nil {
		t.Fatalf("plan returned error: %v", err)
	}

	args := commandRunner.spec.Args
	joined := strings.Join(args, " ")
	if commandRunner.spec.UsePTY {
		t.Fatal("planner should not use PTY")
	}
	mustContain := []string{
		"--json",
		"--sandbox read-only",
		"--output-schema",
		"--output-last-message",
		"--ephemeral",
		"approval_policy",
		"Plan the migration",
	}
	for _, fragment := range mustContain {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("planner args missing %q: %s", fragment, joined)
		}
	}
	if strings.Contains(joined, "workspace-write") {
		t.Fatalf("planner must not run with workspace-write: %s", joined)
	}
	if got := string(resp.Manifest); got == "" {
		t.Fatal("expected manifest from output schema")
	}
	if !strings.Contains(resp.Output, `"name":"test-plan"`) {
		t.Fatalf("expected output from last-message file, got %q", resp.Output)
	}
}
