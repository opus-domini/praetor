package loop

import (
	"context"
	"strings"
	"testing"
)

func TestComposedRuntimeUnsupportedAgent(t *testing.T) {
	t.Parallel()

	rt := newComposedRuntime(defaultAgents(), &directRunner{})
	_, err := rt.Run(context.Background(), AgentRequest{
		Agent:  Agent("unknown-agent"),
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected unsupported agent error")
	}
	if !strings.Contains(err.Error(), "unsupported agent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComposedRuntimeSessionManagerDelegation(t *testing.T) {
	t.Parallel()

	// directRunner does not implement SessionManager.
	rt := newComposedRuntime(defaultAgents(), &directRunner{})
	if err := rt.EnsureSession(); err != nil {
		t.Fatalf("expected no error from non-session runner: %v", err)
	}
	if name := rt.SessionName(); name != "" {
		t.Fatalf("expected empty session name from non-session runner, got %q", name)
	}
	// Cleanup should be a no-op.
	rt.Cleanup()
}

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
		prefix string
		want   string
	}{
		{"executor", "praetor-executor"},
		{"", "praetor-agent"},
		{"a/b:c d", "praetor-a-b-c-d"},
	}
	for _, tt := range tests {
		got := tmuxWindowName(tt.prefix)
		if got != tt.want {
			t.Errorf("tmuxWindowName(%q) = %q, want %q", tt.prefix, got, tt.want)
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
