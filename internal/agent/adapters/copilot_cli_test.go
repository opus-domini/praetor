package adapters

import (
	"context"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type copilotCaptureRunner struct {
	specs  []runner.CommandSpec
	result runner.CommandResult
	err    error
}

func (r *copilotCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.specs = append(r.specs, spec)
	return r.result, r.err
}

func TestCopilotExecuteBuildsExpectedCommand(t *testing.T) {
	t.Parallel()

	commandRunner := &copilotCaptureRunner{result: runner.CommandResult{
		Stdout:   "RESULT: PASS",
		ExitCode: 0,
		Strategy: "process",
	}}
	provider := NewCopilotCLI("copilot", commandRunner)

	resp, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:       "Implement feature",
		SystemPrompt: "Follow project conventions",
		Workdir:      ".",
		Model:        "gpt-4.1",
		OneShot:      true,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.TrimSpace(resp.Output) != "RESULT: PASS" {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
	if len(commandRunner.specs) != 1 {
		t.Fatalf("expected 1 command invocation, got %d", len(commandRunner.specs))
	}
	args := strings.Join(commandRunner.specs[0].Args, " ")
	if !strings.Contains(args, "copilot -p") {
		t.Fatalf("expected copilot -p, got %q", args)
	}
	if !strings.Contains(args, "--allow-all-tools") {
		t.Fatalf("expected --allow-all-tools, got %q", args)
	}
	if !strings.Contains(args, "--model") {
		t.Fatalf("expected --model, got %q", args)
	}
}
