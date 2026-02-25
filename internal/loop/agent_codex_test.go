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

func TestCodexAgentParseOutputJSON(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	output, cost, err := agent.ParseOutput(`{"result":"all done","total_cost_usd":0.42}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "all done" {
		t.Fatalf("expected output='all done', got %q", output)
	}
	if cost != 0.42 {
		t.Fatalf("expected cost=0.42, got %f", cost)
	}
}

func TestCodexAgentParseOutputPlainText(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	output, cost, err := agent.ParseOutput("just plain text output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "just plain text output" {
		t.Fatalf("expected plain text pass-through, got %q", output)
	}
	if cost != 0 {
		t.Fatalf("expected zero cost, got %f", cost)
	}
}

func TestCodexAgentParseOutputInvalidJSON(t *testing.T) {
	t.Parallel()

	agent := &codexAgent{}
	output, cost, err := agent.ParseOutput("{invalid json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "{invalid json" {
		t.Fatalf("expected raw output on invalid JSON, got %q", output)
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
