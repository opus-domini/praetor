package adapters

import (
	"context"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type geminiCaptureRunner struct {
	spec   runner.CommandSpec
	result runner.CommandResult
}

func (r *geminiCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.spec = spec
	return r.result, nil
}

func TestGeminiExecuteOneShotUsesArgs(t *testing.T) {
	t.Parallel()

	commandRunner := &geminiCaptureRunner{result: runner.CommandResult{Stdout: "OK"}}
	provider := NewGeminiCLI("gemini", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: true,
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if commandRunner.spec.UsePTY {
		t.Error("one-shot should not use PTY")
	}
	if commandRunner.spec.Stdin != "" {
		t.Error("one-shot should not use stdin (prompt goes as argument)")
	}
	if !strings.Contains(strings.Join(commandRunner.spec.Args, " "), "say hi") {
		t.Error("one-shot should include prompt as argument")
	}
}

func TestGeminiExecutePipelineUsesPTYAndStdin(t *testing.T) {
	t.Parallel()

	commandRunner := &geminiCaptureRunner{result: runner.CommandResult{Stdout: "OK"}}
	provider := NewGeminiCLI("gemini", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: false,
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if !commandRunner.spec.UsePTY {
		t.Error("pipeline should use PTY")
	}
	if commandRunner.spec.Stdin == "" {
		t.Error("pipeline should pass prompt via stdin")
	}
}
