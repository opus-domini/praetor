package adapters

import (
	"context"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type claudeCaptureRunner struct {
	spec   runner.CommandSpec
	result runner.CommandResult
}

func (r *claudeCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.spec = spec
	return r.result, nil
}

func TestClaudeExecuteOneShotUsesJSONFormat(t *testing.T) {
	t.Parallel()

	commandRunner := &claudeCaptureRunner{result: runner.CommandResult{
		Stdout: `{"result":"OK","model":"claude-sonnet-4-20250514","cost_usd":0.003}`,
	}}
	provider := NewClaudeCLI("claude", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: true,
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	args := strings.Join(commandRunner.spec.Args, " ")
	if commandRunner.spec.UsePTY {
		t.Error("one-shot should not use PTY")
	}
	if commandRunner.spec.Stdin != "" {
		t.Error("one-shot should not use stdin (prompt goes as argument)")
	}
	if !strings.Contains(args, "--output-format json") {
		t.Error("one-shot should use --output-format json")
	}
	if strings.Contains(args, "stream-json") {
		t.Error("one-shot should not use stream-json")
	}
	if strings.Contains(args, "--dangerously-skip-permissions") {
		t.Error("one-shot should not include --dangerously-skip-permissions")
	}
	if !strings.Contains(args, "say hi") {
		t.Error("one-shot should include prompt as argument")
	}
}

func TestClaudeExecutePipelineUsesStreamJSON(t *testing.T) {
	t.Parallel()

	commandRunner := &claudeCaptureRunner{result: runner.CommandResult{
		Stdout: `{"type":"result","result":"OK","cost_usd":0.003}`,
	}}
	provider := NewClaudeCLI("claude", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: false,
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	args := strings.Join(commandRunner.spec.Args, " ")
	if !commandRunner.spec.UsePTY {
		t.Error("pipeline should use PTY")
	}
	if !strings.Contains(args, "stream-json") {
		t.Error("pipeline should use stream-json output format")
	}
	if !strings.Contains(args, "--dangerously-skip-permissions") {
		t.Error("pipeline should include --dangerously-skip-permissions")
	}
	if commandRunner.spec.Stdin == "" {
		t.Error("pipeline should pass prompt via stdin")
	}
}
