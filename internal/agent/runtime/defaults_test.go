package runtime

import (
	"reflect"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
)

func TestDefaultRegistryIncludesBuiltins(t *testing.T) {
	t.Parallel()

	registry := NewDefaultRegistry(DefaultOptions{
		CodexBin:         "codex",
		ClaudeBin:        "claude",
		CopilotBin:       "copilot",
		GeminiBin:        "gemini",
		KimiBin:          "kimi",
		OpenCodeBin:      "opencode",
		OpenRouterURL:    "https://openrouter.ai/api/v1",
		OpenRouterModel:  "openai/gpt-4o-mini",
		OpenRouterKeyEnv: "OPENROUTER_API_KEY",
		OllamaURL:        "http://127.0.0.1:11434",
		OllamaModel:      "llama3",
	})

	got := registry.IDs()
	want := []agent.ID{agent.Claude, agent.Codex, agent.Copilot, agent.Gemini, agent.Kimi, agent.Ollama, agent.OpenCode, agent.OpenRouter}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected default registry IDs:\n got: %v\nwant: %v", got, want)
	}
}
