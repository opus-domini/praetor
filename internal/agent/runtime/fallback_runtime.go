package runtime

import (
	"context"
	"fmt"
	"log"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/domain"
)

// FallbackRuntime wraps a RegistryRuntime and retries with an alternate agent
// when the primary fails with a classified error matching the fallback policy.
type FallbackRuntime struct {
	inner  *RegistryRuntime
	policy agent.FallbackPolicy
}

// NewFallbackRuntime creates a fallback-aware runtime.
// If the policy is empty, the inner runtime is used directly (no overhead).
func NewFallbackRuntime(inner *RegistryRuntime, policy agent.FallbackPolicy) *FallbackRuntime {
	return &FallbackRuntime{inner: inner, policy: policy}
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
