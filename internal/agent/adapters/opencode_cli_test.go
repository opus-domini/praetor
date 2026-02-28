package adapters

import (
	"context"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type opencodeCaptureRunner struct {
	specs  []runner.CommandSpec
	result runner.CommandResult
	err    error
}

func (r *opencodeCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.specs = append(r.specs, spec)
	return r.result, r.err
}

func TestOpenCodeExecuteBuildsExpectedCommand(t *testing.T) {
	t.Parallel()

	commandRunner := &opencodeCaptureRunner{result: runner.CommandResult{
		Stdout:   "RESULT: PASS",
		ExitCode: 0,
		Strategy: "process",
	}}
	provider := NewOpenCodeCLI("opencode", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "Refactor service",
		Workdir: ".",
		Model:   "openai/gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(commandRunner.specs) != 1 {
		t.Fatalf("expected 1 command invocation, got %d", len(commandRunner.specs))
	}
	args := commandRunner.specs[0].Args
	if len(args) < 4 {
		t.Fatalf("unexpected args: %v", args)
	}
	if args[0] != "opencode" || args[1] != "run" || args[2] != "--quiet" {
		t.Fatalf("unexpected command args: %v", args)
	}
}
