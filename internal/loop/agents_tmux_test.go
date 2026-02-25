package loop

import (
	"strings"
	"testing"
)

func TestBuildWrapperScriptRejectsUnsupportedAgent(t *testing.T) {
	t.Parallel()

	_, err := buildWrapperScript(
		AgentRequest{Agent: Agent("unknown-agent")},
		"/tmp/prompt",
		"/tmp/system",
		"/tmp/stdout",
		"/tmp/stderr",
		"/tmp/exit",
		"channel",
	)
	if err == nil {
		t.Fatal("expected unsupported agent error")
	}
}

func TestBuildWrapperScriptCodexIncludesSafetyFlags(t *testing.T) {
	t.Parallel()

	script, err := buildWrapperScript(
		AgentRequest{Agent: AgentCodex, Workdir: "/tmp/workdir"},
		"/tmp/prompt",
		"/tmp/system",
		"/tmp/stdout",
		"/tmp/stderr",
		"/tmp/exit",
		"channel",
	)
	if err != nil {
		t.Fatalf("build wrapper script: %v", err)
	}

	expected := []string{
		"set -euo pipefail",
		"--sandbox workspace-write",
		"--skip-git-repo-check",
		`approval_policy="never"`,
		`--cd "$WORKDIR"`,
	}
	for _, needle := range expected {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q", needle)
		}
	}
}
