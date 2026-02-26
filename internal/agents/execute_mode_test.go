package agents

import (
	"context"
	"strings"
	"testing"
)

// captureRunner records the last CommandSpec passed to Run.
type captureRunner struct {
	spec   CommandSpec
	result CommandResult
}

func (r *captureRunner) Run(_ context.Context, spec CommandSpec) (CommandResult, error) {
	r.spec = spec
	return r.result, nil
}

// --- Codex ---

func TestCodexExecuteOneShotOmitsPipelineFlags(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{result: CommandResult{
		Stdout: `{"type":"item.completed","item":{"id":"1","type":"agent_message","text":"OK"}}`,
	}}
	agent := NewCodexCLI("codex", runner)

	_, err := agent.Execute(context.Background(), ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	args := strings.Join(runner.spec.Args, " ")
	if runner.spec.UsePTY {
		t.Error("OneShot should not use PTY")
	}
	if strings.Contains(args, "--sandbox") {
		t.Error("OneShot should not include --sandbox flag")
	}
	if strings.Contains(args, "approval_policy") {
		t.Error("OneShot should not include approval_policy config")
	}
	if !strings.Contains(args, "--json") {
		t.Error("OneShot should include --json flag")
	}
}

func TestCodexExecutePipelineIncludesSandboxFlags(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{result: CommandResult{
		Stdout: `{"type":"item.completed","item":{"id":"1","type":"agent_message","text":"OK"}}`,
	}}
	agent := NewCodexCLI("codex", runner)

	_, err := agent.Execute(context.Background(), ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: false,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	args := strings.Join(runner.spec.Args, " ")
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

// --- Claude ---

func TestClaudeExecuteOneShotUsesJSONFormat(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{result: CommandResult{
		Stdout: `{"result":"OK","model":"claude-sonnet-4-20250514","cost_usd":0.003}`,
	}}
	agent := NewClaudeCLI("claude", runner)

	_, err := agent.Execute(context.Background(), ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	args := strings.Join(runner.spec.Args, " ")
	if runner.spec.UsePTY {
		t.Error("OneShot should not use PTY")
	}
	if runner.spec.Stdin != "" {
		t.Error("OneShot should not use stdin (prompt goes as argument)")
	}
	if !strings.Contains(args, "--output-format json") {
		t.Error("OneShot should use --output-format json")
	}
	if strings.Contains(args, "stream-json") {
		t.Error("OneShot should not use stream-json")
	}
	if strings.Contains(args, "--dangerously-skip-permissions") {
		t.Error("OneShot should not include --dangerously-skip-permissions")
	}
	if !strings.Contains(args, "say hi") {
		t.Error("OneShot should include prompt as argument")
	}
}

func TestClaudeExecutePipelineUsesStreamJSON(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{result: CommandResult{
		Stdout: `{"type":"result","result":"OK","cost_usd":0.003}`,
	}}
	agent := NewClaudeCLI("claude", runner)

	_, err := agent.Execute(context.Background(), ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: false,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	args := strings.Join(runner.spec.Args, " ")
	if !runner.spec.UsePTY {
		t.Error("pipeline should use PTY")
	}
	if !strings.Contains(args, "stream-json") {
		t.Error("pipeline should use stream-json output format")
	}
	if !strings.Contains(args, "--dangerously-skip-permissions") {
		t.Error("pipeline should include --dangerously-skip-permissions")
	}
	if runner.spec.Stdin == "" {
		t.Error("pipeline should pass prompt via stdin")
	}
}

// --- Gemini ---

func TestGeminiExecuteOneShotNosPTYPromptAsArg(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{result: CommandResult{Stdout: "OK"}}
	agent := NewGeminiCLI("gemini", runner)

	_, err := agent.Execute(context.Background(), ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: true,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if runner.spec.UsePTY {
		t.Error("OneShot should not use PTY")
	}
	if runner.spec.Stdin != "" {
		t.Error("OneShot should not use stdin (prompt goes as argument)")
	}
	if !strings.Contains(strings.Join(runner.spec.Args, " "), "say hi") {
		t.Error("OneShot should include prompt as argument")
	}
}

func TestGeminiExecutePipelineUsesPTYAndStdin(t *testing.T) {
	t.Parallel()

	runner := &captureRunner{result: CommandResult{Stdout: "OK"}}
	agent := NewGeminiCLI("gemini", runner)

	_, err := agent.Execute(context.Background(), ExecuteRequest{
		Prompt:  "say hi",
		Workdir: ".",
		OneShot: false,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !runner.spec.UsePTY {
		t.Error("pipeline should use PTY")
	}
	if runner.spec.Stdin == "" {
		t.Error("pipeline should pass prompt via stdin")
	}
}
