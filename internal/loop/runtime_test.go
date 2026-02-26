package loop

import (
	"strings"
	"testing"
)

func TestBuildTmuxWrapperScriptContainsCommand(t *testing.T) {
	t.Parallel()

	spec := CommandSpec{
		Args: []string{"codex", "exec", "--json", "--sandbox", "workspace-write"},
		Dir:  "/tmp/workdir",
	}
	script := buildTmuxWrapperScript(spec, "", "/tmp/stdout", "/tmp/stderr", "/tmp/exit", "ch-1")

	expected := []string{
		"set -euo pipefail",
		"'codex'",
		"'exec'",
		"'--json'",
		"'--sandbox'",
		"'workspace-write'",
		"codex exec --json --sandbox workspace-write", // banner
	}
	for _, needle := range expected {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q, got:\n%s", needle, script)
		}
	}
}

func TestBuildTmuxWrapperScriptWithStdin(t *testing.T) {
	t.Parallel()

	spec := CommandSpec{
		Args: []string{"claude", "-p"},
		Dir:  "/tmp",
	}
	script := buildTmuxWrapperScript(spec, "/tmp/stdin.txt", "/tmp/stdout", "/tmp/stderr", "/tmp/exit", "ch-2")

	if !strings.Contains(script, "< '/tmp/stdin.txt'") {
		t.Fatalf("expected stdin redirection in script, got:\n%s", script)
	}
}

func TestBuildTmuxWrapperScriptWithEnv(t *testing.T) {
	t.Parallel()

	spec := CommandSpec{
		Args: []string{"test-cmd"},
		Env:  []string{"FOO=bar"},
		Dir:  "/tmp",
	}
	script := buildTmuxWrapperScript(spec, "", "/tmp/stdout", "/tmp/stderr", "/tmp/exit", "ch-3")

	if !strings.Contains(script, "export 'FOO=bar'") {
		t.Fatalf("expected env export in script, got:\n%s", script)
	}
}

func TestTmuxWindowName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		taskLabel string
		role      string
		want      string
	}{
		{"", "executor", "praetor-executor"},
		{"", "", "praetor-agent"},
		{"", "a/b:c d", "praetor-a-b-c-d"},
		{"TASK-001", "executor", "TASK-001-executor"},
		{"TASK-001", "reviewer", "TASK-001-reviewer"},
		{"my feature", "executor", "my-feature-executor"},
		{"#3", "executor", "#3-executor"},
	}
	for _, tt := range tests {
		got := tmuxWindowName(tt.taskLabel, tt.role)
		if got != tt.want {
			t.Errorf("tmuxWindowName(%q, %q) = %q, want %q", tt.taskLabel, tt.role, got, tt.want)
		}
	}
}

func TestTailText(t *testing.T) {
	t.Parallel()

	if got := tailText("", 5); got != "no stderr output" {
		t.Fatalf("expected 'no stderr output', got %q", got)
	}

	if got := tailText("line1\nline2", 5); got != "line1 | line2" {
		t.Fatalf("expected 'line1 | line2', got %q", got)
	}

	longText := strings.Join([]string{"1", "2", "3", "4", "5"}, "\n")
	if got := tailText(longText, 2); got != "4 | 5" {
		t.Fatalf("expected '4 | 5', got %q", got)
	}
}
