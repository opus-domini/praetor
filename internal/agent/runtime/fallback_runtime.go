package runtime

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
)

// FallbackRuntime wraps a RegistryRuntime and retries with an alternate agent
// when the primary fails with a classified error matching the fallback policy.
type FallbackRuntime struct {
	inner  *RegistryRuntime
	policy agent.FallbackPolicy
	sink   middleware.EventSink
}

// NewFallbackRuntime creates a fallback-aware runtime.
// If the policy is empty, the inner runtime is used directly (no overhead).
func NewFallbackRuntime(inner *RegistryRuntime, policy agent.FallbackPolicy, sinks ...middleware.EventSink) *FallbackRuntime {
	var sink middleware.EventSink
	if len(sinks) > 0 {
		sink = sinks[0]
	}
	if sink == nil {
		sink = middleware.NopSink{}
	}
	return &FallbackRuntime{inner: inner, policy: policy, sink: sink}
}

// Run implements domain.AgentRuntime.
func (f *FallbackRuntime) Run(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	result, err := f.inner.Run(ctx, req)
	if err == nil {
		return result, nil
	}

	if ctx.Err() != nil {
		return result, err
	}

	primaryID := agent.Normalize(string(domain.NormalizeAgent(req.Agent)))
	class := agent.ClassifyError(err)
	fallbackID, ok := f.policy.Resolve(primaryID, class)
	if !ok {
		return result, err
	}

	log.Printf("WARN: agent %q failed (%s: %v), falling back to %q", req.Agent, class, err, fallbackID)
	if f.sink != nil {
		f.sink.Emit(middleware.ExecutionEvent{
			SchemaVersion: 1,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Type:          middleware.EventAgentFallback,
			EventType:     string(middleware.EventAgentFallback),
			Agent:         string(req.Agent),
			TaskID:        req.TaskLabel,
			Phase:         req.Role,
			Role:          req.Role,
			Error:         err.Error(),
			Message:       fmt.Sprintf("fallback to %s", fallbackID),
		})
	}

	fallbackReq := req
	fallbackReq.Agent = domain.Agent(fallbackID)
	return f.inner.Run(ctx, fallbackReq)
}

// EnsureSession delegates to the inner runtime's session manager.
func (f *FallbackRuntime) EnsureSession() error {
	if f == nil || f.inner == nil {
		return nil
	}
	return f.inner.EnsureSession()
}

// Cleanup delegates to the inner runtime's session manager.
func (f *FallbackRuntime) Cleanup() {
	if f == nil || f.inner == nil {
		return
	}
	f.inner.Cleanup()
}

// SessionName delegates to the inner runtime's session manager.
func (f *FallbackRuntime) SessionName() string {
	if f == nil || f.inner == nil {
		return ""
	}
	return f.inner.SessionName()
}

// String returns a human-readable description for diagnostics.
func (f *FallbackRuntime) String() string {
	return fmt.Sprintf("FallbackRuntime(policy=%+v)", f.policy)
}
