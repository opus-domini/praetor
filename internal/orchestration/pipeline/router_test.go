package pipeline

import (
	"testing"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/domain"
)

func TestRouterTaskLevelExecutor(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{Executor: "gemini"}
	got, err := resolveExecutorWithRouting(task, domain.AgentCodex, []agent.ID{agent.Claude, agent.Codex})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.AgentGemini {
		t.Fatalf("expected task-level executor gemini, got %q", got)
	}
}

func TestRouterTaskLevelInvalid(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{Executor: "nonexistent"}
	_, err := resolveExecutorWithRouting(task, domain.AgentCodex, nil)
	if err == nil {
		t.Fatal("expected error for invalid task executor")
	}
}

func TestRouterDefaultExecutorAvailable(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{}
	got, err := resolveExecutorWithRouting(task, domain.AgentCodex, []agent.ID{agent.Codex, agent.Claude})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.AgentCodex {
		t.Fatalf("expected default executor codex, got %q", got)
	}
}

func TestRouterDefaultExecutorUnavailableFallsToAvailable(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{}
	got, err := resolveExecutorWithRouting(task, domain.AgentCodex, []agent.ID{agent.Claude, agent.Ollama})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Claude is CLI and should be preferred over Ollama (REST)
	if got != domain.AgentClaude {
		t.Fatalf("expected CLI agent claude, got %q", got)
	}
}

func TestRouterPrefersCLIOverREST(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{}
	got, err := resolveExecutorWithRouting(task, "unavailable", []agent.ID{agent.Ollama, agent.Claude})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.AgentClaude {
		t.Fatalf("expected CLI agent claude over REST ollama, got %q", got)
	}
}

func TestRouterOnlyRESTAvailable(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{}
	got, err := resolveExecutorWithRouting(task, "unavailable", []agent.ID{agent.Ollama})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.AgentOllama {
		t.Fatalf("expected ollama as only available, got %q", got)
	}
}

func TestRouterNoAvailableAgents(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{}
	_, err := resolveExecutorWithRouting(task, "unavailable", []agent.ID{})
	if err == nil {
		t.Fatal("expected error when no available agents and default unavailable")
	}
}

func TestRouterNoAvailabilityDataFallsToResolveExecutor(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{}
	got, err := resolveExecutorWithRouting(task, domain.AgentCodex, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.AgentCodex {
		t.Fatalf("expected fallback to resolveExecutor with codex, got %q", got)
	}
}

func TestRouterTaskExecutorNone(t *testing.T) {
	t.Parallel()
	task := domain.StateTask{Executor: "none"}
	_, err := resolveExecutorWithRouting(task, domain.AgentCodex, []agent.ID{agent.Codex})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsAvailable(t *testing.T) {
	t.Parallel()
	available := []agent.ID{agent.Claude, agent.Codex, agent.Ollama}
	if !isAvailable(agent.Claude, available) {
		t.Fatal("expected claude to be available")
	}
	if isAvailable(agent.Gemini, available) {
		t.Fatal("expected gemini to not be available")
	}
	if isAvailable(agent.Claude, nil) {
		t.Fatal("expected false for nil list")
	}
}
