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

func TestClaudeAgentBuildCommandVerbose(t *testing.T) {
	t.Parallel()

	agent := &claudeAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:   AgentClaude,
		Prompt:  "test",
		Workdir: "/tmp",
		Verbose: true,
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
		t.Fatalf("expected --verbose in args, got %v", spec.Args)
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

	agent := &claudeAgent{}
	output, cost, err := agent.ParseOutput("  the review result  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "the review result" {
		t.Fatalf("expected trimmed output, got %q", output)
	}
	if cost != 0 {
		t.Fatalf("expected zero cost, got %f", cost)
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
