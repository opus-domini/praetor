package adapters

import (
	"context"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

type codexCaptureRunner struct {
	spec   runner.CommandSpec
	result runner.CommandResult
}

func (r *codexCaptureRunner) Run(_ context.Context, spec runner.CommandSpec) (runner.CommandResult, error) {
	r.spec = spec
	return r.result, nil
}

func TestCodexExecuteOneShotOmitsPipelineFlags(t *testing.T) {
	t.Parallel()

	commandRunner := &codexCaptureRunner{result: runner.CommandResult{
		Stdout: `{"type":"item.completed","item":{"id":"1","type":"agent_message","text":"OK"}}`,
	}}
	provider := NewCodexCLI("codex", commandRunner)

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
	if strings.Contains(args, "--sandbox") {
		t.Error("one-shot should not include --sandbox flag")
	}
	if strings.Contains(args, "approval_policy") {
		t.Error("one-shot should not include approval_policy config")
	}
	if !strings.Contains(args, "--json") {
		t.Error("one-shot should include --json flag")
	}
}

func TestCodexExecutePipelineIncludesSandboxFlags(t *testing.T) {
	t.Parallel()

	commandRunner := &codexCaptureRunner{result: runner.CommandResult{
		Stdout: `{"type":"item.completed","item":{"id":"1","type":"agent_message","text":"OK"}}`,
	}}
	provider := NewCodexCLI("codex", commandRunner)

	_, err := provider.Execute(context.Background(), agent.ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: false,
	})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	args := strings.Join(commandRunner.spec.Args, " ")
	if !strings.Contains(args, "--sandbox") {
		t.Error("pipeline should include --sandbox flag")
	}
	if !strings.Contains(args, "approval_policy") {
		t.Error("pipeline should include approval_policy config")
	}
	if !strings.Contains(args, "--json") {
		t.Error("pipeline should include --json flag")
	}
}
