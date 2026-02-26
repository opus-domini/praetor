package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// RegistryRuntime routes loop requests through the central agents.Agent abstraction.
type RegistryRuntime struct {
	registry *Registry
}

// NewRegistryRuntime creates a RegistryRuntime wired with default agents and
// an exec-based command runner configured according to opts.
func NewRegistryRuntime(opts domain.RunnerOptions) *RegistryRuntime {
	runner := NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{
		ForcePTY:   opts.RunnerMode == domain.RunnerPTY,
		DisablePTY: opts.RunnerMode == domain.RunnerDirect,
	})
	return &RegistryRuntime{
		registry: NewDefaultRegistry(DefaultOptions{
			CodexBin:    opts.CodexBin,
			ClaudeBin:   opts.ClaudeBin,
			GeminiBin:   opts.GeminiBin,
			OllamaURL:   opts.OllamaURL,
			OllamaModel: opts.OllamaModel,
			Runner:      runner,
		}),
	}
}

// Run implements domain.AgentRuntime.
func (r *RegistryRuntime) Run(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	if r == nil || r.registry == nil {
		return domain.AgentResult{}, fmt.Errorf("registry runtime is not initialized")
	}

	agentID := Normalize(string(domain.NormalizeAgent(req.Agent)))
	agent, ok := r.registry.Get(agentID)
	if !ok {
		return domain.AgentResult{}, fmt.Errorf("unsupported agent %q", req.Agent)
	}

	switch strings.ToLower(strings.TrimSpace(req.Role)) {
	case "plan":
		resp, err := agent.Plan(ctx, PlanRequest{
			Objective:        strings.TrimSpace(req.Prompt),
			WorkspaceContext: strings.TrimSpace(req.SystemPrompt),
			Workdir:          strings.TrimSpace(req.Workdir),
			Model:            strings.TrimSpace(req.Model),
		})
		if err != nil {
			return domain.AgentResult{}, err
		}
		output := strings.TrimSpace(resp.Output)
		if output == "" && len(resp.Manifest) > 0 {
			output = string(resp.Manifest)
		}
		return domain.AgentResult{Output: output, CostUSD: resp.CostUSD, DurationS: resp.DurationS}, nil
	case "review":
		resp, err := agent.Review(ctx, ReviewRequest{
			Prompt:       strings.TrimSpace(req.Prompt),
			SystemPrompt: strings.TrimSpace(req.SystemPrompt),
			Workdir:      strings.TrimSpace(req.Workdir),
			Model:        strings.TrimSpace(req.Model),
		})
		if err != nil {
			return domain.AgentResult{}, err
		}
		return domain.AgentResult{Output: strings.TrimSpace(resp.Output), CostUSD: resp.CostUSD, DurationS: resp.DurationS}, nil
	default:
		resp, err := agent.Execute(ctx, ExecuteRequest{
			Prompt:       strings.TrimSpace(req.Prompt),
			SystemPrompt: strings.TrimSpace(req.SystemPrompt),
			Workdir:      strings.TrimSpace(req.Workdir),
			Model:        strings.TrimSpace(req.Model),
		})
		if err != nil {
			return domain.AgentResult{}, err
		}
		return domain.AgentResult{Output: strings.TrimSpace(resp.Output), CostUSD: resp.CostUSD, DurationS: resp.DurationS}, nil
	}
}
