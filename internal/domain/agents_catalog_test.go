package domain

import "testing"

func TestValidExecutorsIncludesExtendedProviders(t *testing.T) {
	t.Parallel()

	providers := []Agent{
		AgentClaude,
		AgentCodex,
		AgentCopilot,
		AgentGemini,
		AgentKimi,
		AgentOpenCode,
		AgentOpenRouter,
		AgentOllama,
	}
	for _, provider := range providers {
		if _, ok := ValidExecutors[provider]; !ok {
			t.Fatalf("expected provider %q in ValidExecutors", provider)
		}
		if _, ok := ValidReviewers[provider]; !ok {
			t.Fatalf("expected provider %q in ValidReviewers", provider)
		}
	}
}

func TestAgentCatalogBinaryResolution(t *testing.T) {
	t.Parallel()

	opts := RunnerOptions{
		CodexBin:    "codex-bin",
		ClaudeBin:   "claude-bin",
		CopilotBin:  "copilot-bin",
		GeminiBin:   "gemini-bin",
		KimiBin:     "kimi-bin",
		OpenCodeBin: "opencode-bin",
	}

	cases := map[Agent]string{
		AgentCodex:    "codex-bin",
		AgentClaude:   "claude-bin",
		AgentCopilot:  "copilot-bin",
		AgentGemini:   "gemini-bin",
		AgentKimi:     "kimi-bin",
		AgentOpenCode: "opencode-bin",
	}
	for provider, want := range cases {
		got, ok := AgentBinary(opts, provider)
		if !ok {
			t.Fatalf("expected binary mapping for %q", provider)
		}
		if got != want {
			t.Fatalf("AgentBinary(%q) = %q, want %q", provider, got, want)
		}
	}

	if _, ok := AgentBinary(opts, AgentOpenRouter); ok {
		t.Fatal("openrouter should not be treated as binary-backed provider")
	}
	if !AgentRequiresBinary(AgentKimi) {
		t.Fatal("expected kimi to require binary")
	}
	if AgentRequiresBinary(AgentOpenRouter) {
		t.Fatal("expected openrouter to be rest-backed")
	}
}

func TestCapabilitiesForAgentDeriveFromCatalog(t *testing.T) {
	t.Parallel()

	// CLI agents should have RequiresBinary=true.
	cliAgents := []Agent{AgentCodex, AgentClaude, AgentCopilot, AgentGemini, AgentKimi, AgentOpenCode}
	for _, a := range cliAgents {
		caps, ok := CapabilitiesForAgent(a)
		if !ok {
			t.Fatalf("expected capabilities for %q", a)
		}
		if caps.Transport != AgentTransportCLI {
			t.Errorf("expected CLI transport for %q, got %q", a, caps.Transport)
		}
		if !caps.RequiresBinary {
			t.Errorf("expected RequiresBinary=true for CLI agent %q", a)
		}
	}

	// REST agents should have RequiresBinary=false.
	restAgents := []Agent{AgentOpenRouter, AgentOllama}
	for _, a := range restAgents {
		caps, ok := CapabilitiesForAgent(a)
		if !ok {
			t.Fatalf("expected capabilities for %q", a)
		}
		if caps.Transport != AgentTransportREST {
			t.Errorf("expected REST transport for %q, got %q", a, caps.Transport)
		}
		if caps.RequiresBinary {
			t.Errorf("expected RequiresBinary=false for REST agent %q", a)
		}
	}
}

func TestCapabilitiesForUnknownAgent(t *testing.T) {
	t.Parallel()

	_, ok := CapabilitiesForAgent("unknown")
	if ok {
		t.Fatal("expected no capabilities for unknown agent")
	}
}

func TestAgentSupportsTMUXAllAgents(t *testing.T) {
	t.Parallel()

	agents := []Agent{AgentCodex, AgentClaude, AgentCopilot, AgentGemini, AgentKimi, AgentOpenCode, AgentOpenRouter, AgentOllama}
	for _, a := range agents {
		if !AgentSupportsTMUX(a) {
			t.Errorf("expected %q to support TMUX", a)
		}
	}
}

func TestAgentSupportsTMUXNoneAndEmpty(t *testing.T) {
	t.Parallel()

	if !AgentSupportsTMUX(AgentNone) {
		t.Error("expected AgentNone to support TMUX")
	}
	if !AgentSupportsTMUX("") {
		t.Error("expected empty agent to support TMUX")
	}
}

func TestAgentBinaryFallsBackToCatalogDefault(t *testing.T) {
	t.Parallel()

	// Empty RunnerOptions means no overrides — should use catalog default.
	opts := RunnerOptions{}
	got, ok := AgentBinary(opts, AgentClaude)
	if !ok {
		t.Fatal("expected binary for Claude")
	}
	if got != "claude" {
		t.Errorf("expected catalog default 'claude', got %q", got)
	}
}

func TestAgentBinaryRESTAgentReturnsNotOK(t *testing.T) {
	t.Parallel()

	opts := RunnerOptions{}
	_, ok := AgentBinary(opts, AgentOllama)
	if ok {
		t.Fatal("expected REST agent to not have a binary")
	}
}

func TestAgentDisplayNameFromCatalog(t *testing.T) {
	t.Parallel()

	got := AgentDisplayName(AgentClaude)
	if got != "Claude Code" {
		t.Errorf("expected 'Claude Code', got %q", got)
	}

	got = AgentDisplayName(AgentOllama)
	if got != "Ollama" {
		t.Errorf("expected 'Ollama', got %q", got)
	}
}

func TestAgentDisplayNameUnknownFallback(t *testing.T) {
	t.Parallel()

	got := AgentDisplayName("unknown")
	if got != "unknown" {
		t.Errorf("expected 'unknown' fallback, got %q", got)
	}
}

func TestAgentDisplayNameEmpty(t *testing.T) {
	t.Parallel()

	got := AgentDisplayName("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestAgentRequiresBinaryAllProviders(t *testing.T) {
	t.Parallel()

	type testCase struct {
		agent    Agent
		expected bool
	}
	cases := []testCase{
		{AgentCodex, true},
		{AgentClaude, true},
		{AgentCopilot, true},
		{AgentGemini, true},
		{AgentKimi, true},
		{AgentOpenCode, true},
		{AgentOpenRouter, false},
		{AgentOllama, false},
	}
	for _, tc := range cases {
		got := AgentRequiresBinary(tc.agent)
		if got != tc.expected {
			t.Errorf("AgentRequiresBinary(%q) = %v, want %v", tc.agent, got, tc.expected)
		}
	}
}
