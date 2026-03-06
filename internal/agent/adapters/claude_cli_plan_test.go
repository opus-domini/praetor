package adapters

import (
	"context"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

func TestClaudePlanUsesSchemaEnforcedOneShot(t *testing.T) {
	t.Parallel()

	commandRunner := &claudeCaptureRunner{result: runner.CommandResult{
		Stdout: `{"type":"result","model":"claude-sonnet-4-20250514","cost_usd":0.003,"structured_output":{"name":"test-plan","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}}`,
	}}
	provider := NewClaudeCLI("claude", commandRunner)

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
	if commandRunner.spec.Stdin != "" {
		t.Fatal("planner should not use stdin")
	}
	mustContain := []string{
		"--output-format json",
		"--disable-slash-commands",
		"--system-prompt",
		"--json-schema",
		"Plan the migration",
	}
	for _, fragment := range mustContain {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("planner args missing %q: %s", fragment, joined)
		}
	}
	hasToolsDisabled := false
	for i := range args[:len(args)-1] {
		if args[i] == "--tools" && args[i+1] == "" {
			hasToolsDisabled = true
			break
		}
	}
	if !hasToolsDisabled {
		t.Fatalf("planner should disable tools, args=%q", args)
	}
	if strings.Contains(joined, "--permission-mode plan") {
		t.Fatalf("planner must not use Claude plan mode: %s", joined)
	}
	if strings.Contains(joined, "--append-system-prompt") {
		t.Fatalf("planner should override the default system prompt instead of appending to it: %s", joined)
	}
	if got := string(resp.Manifest); got == "" {
		t.Fatal("expected manifest from schema output")
	}
	if resp.Output == "" {
		t.Fatal("expected planner output")
	}
}
