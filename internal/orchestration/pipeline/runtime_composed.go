package pipeline

import (
	"fmt"
	"log"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/middleware"
	agentruntime "github.com/opus-domini/praetor/internal/agent/runtime"
	"github.com/opus-domini/praetor/internal/domain"
)

// composedRuntime preserves SessionManager delegation while wrapping
// the runtime with middleware layers.
type composedRuntime struct {
	domain.AgentRuntime
	session *agentruntime.RegistryRuntime
}

func (c composedRuntime) EnsureSession() error {
	if c.session == nil {
		return nil
	}
	return c.session.EnsureSession()
}

func (c composedRuntime) Cleanup() {
	if c.session == nil {
		return
	}
	c.session.Cleanup()
}

func (c composedRuntime) SessionName() string {
	if c.session == nil {
		return ""
	}
	return c.session.SessionName()
}

// RuntimeDeps holds optional dependencies injected into the composed runtime.
type RuntimeDeps struct {
	EventSink middleware.EventSink
	Counters  *middleware.Counters
	Logger    middleware.Logger
}

// defaultLogger sends log entries to the standard logger.
type defaultLogger struct {
	verbose bool
}

func (l defaultLogger) Log(entry middleware.LogEntry) {
	if entry.Status == "ok" && !l.verbose {
		return
	}
	if entry.Error != "" {
		log.Printf("[%s] agent=%s role=%s status=%s error=%q duration=%.1fs cost=$%.4f",
			entry.Timestamp, entry.Agent, entry.Role, entry.Status, entry.Error, entry.DurationS, entry.CostUSD)
		return
	}
	log.Printf("[%s] agent=%s role=%s status=%s duration=%.1fs cost=$%.4f",
		entry.Timestamp, entry.Agent, entry.Role, entry.Status, entry.DurationS, entry.CostUSD)
}

// BuildAgentRuntime creates the unified agents runtime for all runner modes,
// layering fallback, logging, and metrics middleware.
func BuildAgentRuntime(opts domain.RunnerOptions) (domain.AgentRuntime, error) {
	return BuildAgentRuntimeWithDeps(opts, RuntimeDeps{})
}

// BuildAgentRuntimeWithDeps creates the layered runtime with explicit dependencies.
func BuildAgentRuntimeWithDeps(opts domain.RunnerOptions, deps RuntimeDeps) (domain.AgentRuntime, error) {
	switch opts.RunnerMode {
	case domain.RunnerTMUX, domain.RunnerPTY, domain.RunnerDirect:
	default:
		return nil, fmt.Errorf("unsupported runner mode %q", opts.RunnerMode)
	}

	registry := agentruntime.NewRegistryRuntime(opts)

	// Phase 2: Fallback layer
	var rt domain.AgentRuntime = registry
	policy := buildFallbackPolicy(opts)
	if !policy.IsEmpty() {
		rt = agentruntime.NewFallbackRuntime(registry, policy, deps.EventSink)
	}

	// Phase 3+4: Middleware chain (logging + metrics)
	sink := deps.EventSink
	if sink == nil {
		sink = middleware.NopSink{}
	}
	logger := deps.Logger
	if logger == nil {
		logger = defaultLogger{verbose: opts.Verbose}
	}
	counters := deps.Counters
	if counters == nil {
		counters = middleware.NewCounters()
	}

	rt = middleware.Chain(rt,
		middleware.Logging(logger, sink),
		middleware.Metrics(counters),
	)

	return composedRuntime{
		AgentRuntime: rt,
		session:      registry,
	}, nil
}

func buildFallbackPolicy(opts domain.RunnerOptions) agent.FallbackPolicy {
	policy := agent.FallbackPolicy{}

	if opts.FallbackOnTransient != "" {
		policy.OnTransient = agent.Normalize(string(opts.FallbackOnTransient))
	}
	if opts.FallbackOnAuth != "" {
		policy.OnAuth = agent.Normalize(string(opts.FallbackOnAuth))
	}

	// Per-agent mapping: fallback config maps default executor → fallback agent.
	if opts.FallbackAgent != "" {
		fb := agent.Normalize(string(opts.FallbackAgent))
		executor := agent.Normalize(string(opts.DefaultExecutor))
		if fb != "" && executor != "" {
			policy.Mappings = map[agent.ID]agent.ID{
				executor: fb,
			}
		}
	}

	return policy
}
