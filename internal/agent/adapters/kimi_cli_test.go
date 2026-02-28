package adapters

import (
	"context"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type kimiCaptureRunner struct {
	specs  []runner.CommandSpec
	result runner.CommandResult
	err    error
}

func (r *kimiCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.specs = append(r.specs, spec)
	return r.result, r.err
}

func TestKimiExecuteUsesPTYAndStdin(t *testing.T) {
	t.Parallel()

	commandRunner := &kimiCaptureRunner{result: runner.CommandResult{
		Stdout:   "RESULT: PASS",
		ExitCode: 0,
		Strategy: "pty",
	}}
	provider := NewKimiCLI("kimi", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:       "Run checks",
		SystemPrompt: "Be concise",
		Workdir:      ".",
		OneShot:      true,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(commandRunner.specs) != 1 {
		t.Fatalf("expected 1 command invocation, got %d", len(commandRunner.specs))
	}
	spec := commandRunner.specs[0]
	if !spec.UsePTY {
		t.Fatal("expected kimi execution to request PTY")
	}
	if !strings.Contains(spec.Stdin, "Run checks") {
		t.Fatalf("expected prompt in stdin, got %q", spec.Stdin)
	}
}
