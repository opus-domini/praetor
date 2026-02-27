package pipeline

import (
	"fmt"

	agentruntime "github.com/opus-domini/praetor/internal/agent/runtime"
	"github.com/opus-domini/praetor/internal/domain"
)

// BuildAgentRuntime creates the unified agents runtime for all runner modes.
func BuildAgentRuntime(opts domain.RunnerOptions) (domain.AgentRuntime, error) {
	switch opts.RunnerMode {
	case domain.RunnerTMUX, domain.RunnerPTY, domain.RunnerDirect:
		return agentruntime.NewRegistryRuntime(opts), nil
	default:
		return nil, fmt.Errorf("unsupported runner mode %q", opts.RunnerMode)
	}
}
