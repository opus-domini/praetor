package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
	processruntime "github.com/opus-domini/praetor/internal/runtime/process"
)

func TestComposedRuntimeUnsupportedAgent(t *testing.T) {
	t.Parallel()

	rt := newComposedRuntime(defaultAgents(), &processruntime.Runner{})
	_, err := rt.Run(context.Background(), domain.AgentRequest{
		Agent:  domain.Agent("unknown-agent"),
		Prompt: "test",
	})
	if err == nil {
		t.Fatal("expected unsupported agent error")
	}
	if !strings.Contains(err.Error(), "unsupported agent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComposedRuntimeSessionManagerDelegation(t *testing.T) {
	t.Parallel()

	// processruntime.Runner does not implement SessionManager.
	rt := newComposedRuntime(defaultAgents(), &processruntime.Runner{})
	if err := rt.EnsureSession(); err != nil {
		t.Fatalf("expected no error from non-session runner: %v", err)
	}
	if name := rt.SessionName(); name != "" {
		t.Fatalf("expected empty session name from non-session runner, got %q", name)
	}
	// Cleanup should be a no-op.
	rt.Cleanup()
}
