package pipeline

import (
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
)

func TestBuildAgentRuntimeReturnsUnifiedRuntime(t *testing.T) {
	t.Parallel()

	runtime, err := BuildAgentRuntime(domain.RunnerOptions{
		RunnerMode:  domain.RunnerDirect,
		CodexBin:    "codex",
		ClaudeBin:   "claude",
		GeminiBin:   "gemini",
		OllamaURL:   "http://127.0.0.1:11434",
		OllamaModel: "llama3",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	if runtime == nil {
		t.Fatal("expected non-nil runtime")
	}
}

func TestBuildAgentRuntimeTMUXSessionName(t *testing.T) {
	t.Parallel()

	runtime, err := BuildAgentRuntime(domain.RunnerOptions{
		RunnerMode:  domain.RunnerTMUX,
		TMUXSession: "praetor-test",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	sm, ok := runtime.(domain.SessionManager)
	if !ok {
		t.Fatal("expected runtime to implement SessionManager")
	}
	if got := sm.SessionName(); got != "praetor-test" {
		t.Fatalf("unexpected tmux session name: %q", got)
	}
}

func TestBuildAgentRuntimeRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()

	_, err := BuildAgentRuntime(domain.RunnerOptions{RunnerMode: domain.RunnerMode("invalid")})
	if err == nil {
		t.Fatal("expected unsupported runner mode error")
	}
	if !strings.Contains(err.Error(), "unsupported runner mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}
