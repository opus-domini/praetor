package loop

import (
	"strings"
	"testing"
)

func TestCodexAgentBuildCommand(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:   AgentCodex,
		Prompt:  "implement the feature",
		Workdir: "/tmp/workdir",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{
		"codex", "exec", "--json",
		"--sandbox", "workspace-write",
		"--skip-git-repo-check",
		"--config", `approval_policy="never"`,
		"--cd", "/tmp/workdir",
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

	// Prompt should be last positional argument.
	last := spec.Args[len(spec.Args)-1]
	if last != "implement the feature" {
		t.Fatalf("expected prompt as last arg, got %q", last)
	}

	if spec.Stdin != "" {
		t.Fatal("codex should not use stdin")
	}
}

func TestCodexAgentBuildCommandWithModel(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:   AgentCodex,
		Prompt:  "test",
		Workdir: "/tmp",
		Model:   "o3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundModel := false
	for i, arg := range spec.Args {
		if arg == "--model" && i+1 < len(spec.Args) && spec.Args[i+1] == "o3" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Fatalf("expected --model o3 in args, got %v", spec.Args)
	}
}

func TestCodexAgentBuildCommandWithSystemPrompt(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:        AgentCodex,
		Prompt:       "do the work",
		SystemPrompt: "you are helpful",
		Workdir:      "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// System prompt should be prepended to the final prompt argument.
	last := spec.Args[len(spec.Args)-1]
	if !strings.HasPrefix(last, "you are helpful") {
		t.Fatalf("expected system prompt prepended, got %q", last)
	}
	if !strings.Contains(last, "do the work") {
		t.Fatalf("expected prompt in last arg, got %q", last)
	}
}

func TestCodexAgentParseOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stdout     string
		wantOutput string
		wantCost   float64
	}{
		{
			name: "JSONL with agent_message",
			stdout: `{"type":"thread.started","thread_id":"abc123"}` + "\n" +
				`{"type":"turn.started"}` + "\n" +
				`{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"thinking..."}}` + "\n" +
				`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Hello!"}}` + "\n" +
				`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":10}}`,
			wantOutput: "Hello!",
			wantCost:   0,
		},
		{
			name: "multiple agent_message events joined",
			stdout: `{"type":"item.completed","item":{"type":"agent_message","text":"Part one."}}` + "\n" +
				`{"type":"item.completed","item":{"type":"agent_message","text":"Part two."}}`,
			wantOutput: "Part one.\nPart two.",
			wantCost:   0,
		},
		{
			name:       "plain text fallback",
			stdout:     "just plain text output",
			wantOutput: "just plain text output",
			wantCost:   0,
		},
		{
			name:       "empty output",
			stdout:     "",
			wantOutput: "",
			wantCost:   0,
		},
		{
			name:       "invalid JSON fallback",
			stdout:     "{invalid json",
			wantOutput: "{invalid json",
			wantCost:   0,
		},
	}

	agent := &codexAgent{}
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

func TestCodexAgentString(t *testing.T) {
	t.Parallel()
	agent := &codexAgent{}
	if s := agent.String(); s == "" {
		t.Fatal("expected non-empty string")
	}
}

func TestCodexAgentParseOutputJSONLNoMessages(t *testing.T) {
	t.Parallel()

	// Valid JSONL but no agent_message events — returns raw stdout.
	agent := &codexAgent{}
	stdout := `{"type":"thread.started","thread_id":"abc123"}` + "\n" +
		`{"type":"turn.started"}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":100,"output_tokens":10}}`
	output, cost, err := agent.ParseOutput(stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != stdout {
		t.Fatalf("expected raw stdout when no agent messages, got %q", output)
	}
	if cost != 0 {
		t.Fatalf("expected zero cost, got %f", cost)
	}
}

func TestCodexAgentBuildCommandCustomBin(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	spec, err := agent.BuildCommand(AgentRequest{
		Agent:    AgentCodex,
		Prompt:   "test",
		Workdir:  "/tmp",
		CodexBin: "/opt/bin/codex",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Args[0] != "/opt/bin/codex" {
		t.Fatalf("expected custom binary path, got %q", spec.Args[0])
	}
}
