package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
)

// fakeAgent is a minimal agent.Agent for testing.
type fakeAgent struct {
	id      agent.ID
	execErr error
	execOut string
}

func (f *fakeAgent) ID() agent.ID                     { return f.id }
func (f *fakeAgent) Capabilities() agent.Capabilities { return agent.Capabilities{} }
func (f *fakeAgent) Plan(_ context.Context, _ agent.PlanRequest) (agent.PlanResponse, error) {
	return agent.PlanResponse{}, nil
}
func (f *fakeAgent) Execute(_ context.Context, _ agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	if f.execErr != nil {
		return agent.ExecuteResponse{}, f.execErr
	}
	return agent.ExecuteResponse{Output: f.execOut}, nil
}
func (f *fakeAgent) Review(_ context.Context, _ agent.ReviewRequest) (agent.ReviewResponse, error) {
	return agent.ReviewResponse{}, nil
}

func newTestRegistryRuntime(agents ...agent.Agent) *RegistryRuntime {
	reg := agent.NewRegistry()
	for _, a := range agents {
		_ = reg.Register(a)
	}
	return &RegistryRuntime{registry: reg}
}

func TestFallbackRuntimeSuccess(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execOut: "hello"}
	inner := newTestRegistryRuntime(primary)
	policy := agent.FallbackPolicy{OnTransient: agent.Ollama}
	rt := NewFallbackRuntime(inner, policy)

	result, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "hello" {
		t.Fatalf("expected output=hello, got %q", result.Output)
	}
}

func TestFallbackRuntimeTransientFallback(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("connection refused")}
	fallback := &fakeAgent{id: agent.Ollama, execOut: "fallback-ok"}
	inner := newTestRegistryRuntime(primary, fallback)
	policy := agent.FallbackPolicy{OnTransient: agent.Ollama}
	rt := NewFallbackRuntime(inner, policy)

	result, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}
	if result.Output != "fallback-ok" {
		t.Fatalf("expected fallback output, got %q", result.Output)
	}
}

func TestFallbackRuntimeNoPolicy(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("connection refused")}
	inner := newTestRegistryRuntime(primary)
	policy := agent.FallbackPolicy{}
	rt := NewFallbackRuntime(inner, policy)

	_, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err == nil {
		t.Fatal("expected original error when no fallback configured")
	}
}

func TestFallbackRuntimeUnmatchedErrorClass(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("some random error")}
	fallback := &fakeAgent{id: agent.Ollama, execOut: "never"}
	inner := newTestRegistryRuntime(primary, fallback)
	policy := agent.FallbackPolicy{OnTransient: agent.Ollama}
	rt := NewFallbackRuntime(inner, policy)

	_, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err == nil {
		t.Fatal("expected original error for unknown error class")
	}
}

func TestFallbackRuntimeContextCanceled(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("connection refused")}
	fallback := &fakeAgent{id: agent.Ollama, execOut: "should-not-reach"}
	inner := newTestRegistryRuntime(primary, fallback)
	policy := agent.FallbackPolicy{OnTransient: agent.Ollama}
	rt := NewFallbackRuntime(inner, policy)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Run(ctx, domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err == nil {
		t.Fatal("expected error when context is canceled")
	}
}

func TestFallbackRuntimePerAgentMapping(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("HTTP 503 Service Unavailable")}
	mapped := &fakeAgent{id: agent.Gemini, execOut: "mapped-ok"}
	global := &fakeAgent{id: agent.Ollama, execOut: "global-ok"}
	inner := newTestRegistryRuntime(primary, mapped, global)
	policy := agent.FallbackPolicy{
		Mappings:    map[agent.ID]agent.ID{agent.Claude: agent.Gemini},
		OnTransient: agent.Ollama,
	}
	rt := NewFallbackRuntime(inner, policy)

	result, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err != nil {
		t.Fatalf("expected mapping fallback, got error: %v", err)
	}
	if result.Output != "mapped-ok" {
		t.Fatalf("expected per-agent mapping output, got %q", result.Output)
	}
}

func TestFallbackRuntimeSessionManagerDelegation(t *testing.T) {
	t.Parallel()
	inner := newTestRegistryRuntime()
	policy := agent.FallbackPolicy{}
	rt := NewFallbackRuntime(inner, policy)

	if err := rt.EnsureSession(); err != nil {
		t.Fatalf("unexpected EnsureSession error: %v", err)
	}
	rt.Cleanup()
	if name := rt.SessionName(); name != "" {
		t.Fatalf("expected empty session name, got %q", name)
	}
}

func TestFallbackRuntimeAuthFallback(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("HTTP 401 unauthorized")}
	fallback := &fakeAgent{id: agent.Ollama, execOut: "auth-fallback-ok"}
	inner := newTestRegistryRuntime(primary, fallback)
	policy := agent.FallbackPolicy{OnAuth: agent.Ollama}
	rt := NewFallbackRuntime(inner, policy)

	result, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	if err != nil {
		t.Fatalf("expected auth fallback, got error: %v", err)
	}
	if result.Output != "auth-fallback-ok" {
		t.Fatalf("expected auth fallback output, got %q", result.Output)
	}
}

func TestFallbackRuntimeEmitsFallbackEvent(t *testing.T) {
	t.Parallel()
	primary := &fakeAgent{id: agent.Claude, execErr: fmt.Errorf("connection refused")}
	fallback := &fakeAgent{id: agent.Ollama, execOut: "ok"}
	inner := newTestRegistryRuntime(primary, fallback)
	collector := &middleware.CollectorSink{}
	rt := NewFallbackRuntime(inner, agent.FallbackPolicy{OnTransient: agent.Ollama}, collector)

	if _, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute", TaskLabel: "TASK-001"}); err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if collector.Len() == 0 {
		t.Fatal("expected fallback event emission")
	}
	last := collector.Events[len(collector.Events)-1]
	if last.Type != middleware.EventAgentFallback {
		t.Fatalf("expected agent_fallback event, got %q", last.Type)
	}
	if last.TaskID != "TASK-001" {
		t.Fatalf("expected task id propagation, got %q", last.TaskID)
	}
}
