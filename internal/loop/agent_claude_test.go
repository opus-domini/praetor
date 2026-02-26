package loop

import (
	"strings"
	"testing"
)

func TestClaudeAgentBuildCommand(t *testing.T) {
	t.Parallel()

	agent := &claudeAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:   AgentClaude,
		Prompt:  "review the code",
		Workdir: "/tmp/workdir",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		"claude", "-p",
		"--dangerously-skip-permissions",
		"--no-session-persistence",
		"--verbose",
		"--output-format", "stream-json",
	}
	for _, want := range expected {
		found := false
		for _, got := range spec.Args {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected args to contain %q, got %v", want, spec.Args)
		}
	}

	if spec.Stdin != "review the code" {
		t.Fatalf("expected prompt via stdin, got %q", spec.Stdin)
	}
}

func TestClaudeAgentBuildCommandWithModel(t *testing.T) {
	t.Parallel()

	agent := &claudeAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:   AgentClaude,
		Prompt:  "test",
		Workdir: "/tmp",
		Model:   "sonnet",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundModel := false
	for i, arg := range spec.Args {
		if arg == "--model" && i+1 < len(spec.Args) && spec.Args[i+1] == "sonnet" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Fatalf("expected --model sonnet in args, got %v", spec.Args)
	}
}

func TestClaudeAgentBuildCommandAlwaysVerbose(t *testing.T) {
	t.Parallel()

	agent := &claudeAgent{}

	// --verbose is always present (required by --output-format stream-json).
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:   AgentClaude,
		Prompt:  "test",
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, arg := range spec.Args {
		if arg == "--verbose" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected --verbose in args (required by stream-json), got %v", spec.Args)
	}
}

func TestClaudeAgentBuildCommandWithSystemPrompt(t *testing.T) {
	t.Parallel()

	agent := &claudeAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:        AgentClaude,
		Prompt:       "do work",
		SystemPrompt: "you are helpful",
		Workdir:      "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundFlag := false
	for i, arg := range spec.Args {
		if arg == "--append-system-prompt" && i+1 < len(spec.Args) && spec.Args[i+1] == "you are helpful" {
			foundFlag = true
			break
		}
	}
	if !foundFlag {
		t.Fatalf("expected --append-system-prompt in args, got %v", spec.Args)
	}

	// System prompt should NOT be in stdin for claude.
	if strings.Contains(spec.Stdin, "you are helpful") {
		t.Fatal("system prompt should be via flag, not stdin")
	}
}

func TestClaudeAgentParseOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stdout     string
		wantOutput string
		wantCost   float64
	}{
		{
			name:       "stream-json with result event",
			stdout:     `{"type":"system","subtype":"init"}` + "\n" + `{"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}}` + "\n" + `{"type":"result","subtype":"success","result":"hello world","cost_usd":0.05}`,
			wantOutput: "hello world",
			wantCost:   0.05,
		},
		{
			name:       "result event with empty result falls back to assistant text",
			stdout:     `{"type":"assistant","message":{"content":[{"type":"text","text":"fallback text"}]}}` + "\n" + `{"type":"result","subtype":"success","result":"","cost_usd":0.02}`,
			wantOutput: "fallback text",
			wantCost:   0.02,
		},
		{
			name:       "no result event collects assistant text",
			stdout:     `{"type":"system","subtype":"init"}` + "\n" + `{"type":"assistant","message":{"content":[{"type":"text","text":"only assistant"}]}}`,
			wantOutput: "only assistant",
			wantCost:   0,
		},
		{
			name:       "plain text fallback (not stream-json)",
			stdout:     "  the review result  ",
			wantOutput: "the review result",
			wantCost:   0,
		},
		{
			name:       "empty output",
			stdout:     "",
			wantOutput: "",
			wantCost:   0,
		},
		{
			name:       "multiple assistant events joined",
			stdout:     `{"type":"assistant","message":{"content":[{"type":"text","text":"part one"}]}}` + "\n" + `{"type":"assistant","message":{"content":[{"type":"text","text":"part two"}]}}` + "\n" + `{"type":"result","subtype":"success","result":"","cost_usd":0.01}`,
			wantOutput: "part one\npart two",
			wantCost:   0.01,
		},
		{
			name:       "last result event wins",
			stdout:     `{"type":"result","result":"first","cost_usd":0.01}` + "\n" + `{"type":"result","result":"second","cost_usd":0.03}`,
			wantOutput: "second",
			wantCost:   0.03,
		},
		{
			name:       "skips non-text content blocks",
			stdout:     `{"type":"assistant","message":{"content":[{"type":"tool_use","text":""},{"type":"text","text":"real text"}]}}`,
			wantOutput: "real text",
			wantCost:   0,
		},
		{
			name:       "skips blank lines in stream",
			stdout:     "\n\n" + `{"type":"result","result":"ok","cost_usd":0.01}` + "\n\n",
			wantOutput: "ok",
			wantCost:   0.01,
		},
	}

	agent := &claudeAgent{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			output, cost, err := agent.ParseOutput(tt.stdout)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if output != tt.wantOutput {
				t.Fatalf("output = %q, want %q", output, tt.wantOutput)
			}
			if cost != tt.wantCost {
				t.Fatalf("cost = %f, want %f", cost, tt.wantCost)
			}
		})
	}
}

func TestClaudeAgentString(t *testing.T) {
	t.Parallel()
	agent := &claudeAgent{}
	if s := agent.String(); s == "" {
		t.Fatal("expected non-empty string")
	}
}

func TestClaudeAgentBuildCommandCustomBin(t *testing.T) {
	t.Parallel()

	agent := &claudeAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:     AgentClaude,
		Prompt:    "test",
		Workdir:   "/tmp",
		ClaudeBin: "/opt/bin/claude",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Args[0] != "/opt/bin/claude" {
		t.Fatalf("expected custom binary path, got %q", spec.Args[0])
	}
}
