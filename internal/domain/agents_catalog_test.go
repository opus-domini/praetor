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
