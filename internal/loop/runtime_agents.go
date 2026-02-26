package loop

import (
	"context"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/agents"
)

// registryRuntime routes loop requests through the central agents.Agent abstraction.
type registryRuntime struct {
	registry *agents.Registry
}

func newRegistryRuntime(opts RunnerOptions) *registryRuntime {
	runner := agents.NewExecCommandRunnerWithOptions(agents.ExecCommandRunnerOptions{
		ForcePTY:   opts.RunnerMode == RunnerPTY,
		DisablePTY: opts.RunnerMode == RunnerDirect,
	})
	return &registryRuntime{
		registry: agents.NewDefaultRegistry(agents.DefaultOptions{
			CodexBin:    opts.CodexBin,
			ClaudeBin:   opts.ClaudeBin,
			GeminiBin:   opts.GeminiBin,
			OllamaURL:   opts.OllamaURL,
			OllamaModel: opts.OllamaModel,
			Runner:      runner,
		}),
	}
}

func (r *registryRuntime) Run(ctx context.Context, req AgentRequest) (AgentResult, error) {
	if r == nil || r.registry == nil {
		return AgentResult{}, fmt.Errorf("registry runtime is not initialized")
	}

	agentID := agents.Normalize(string(normalizeAgent(req.Agent)))
	agent, ok := r.registry.Get(agentID)
	if !ok {
		return AgentResult{}, fmt.Errorf("unsupported agent %q", req.Agent)
	}

	switch strings.ToLower(strings.TrimSpace(req.Role)) {
	case "plan":
		resp, err := agent.Plan(ctx, agents.PlanRequest{
			Objective:        strings.TrimSpace(req.Prompt),
			WorkspaceContext: strings.TrimSpace(req.SystemPrompt),
			Workdir:          strings.TrimSpace(req.Workdir),
			Model:            strings.TrimSpace(req.Model),
		})
		if err != nil {
			return AgentResult{}, err
		}
		output := strings.TrimSpace(resp.Output)
		if output == "" && len(resp.Manifest) > 0 {
			output = string(resp.Manifest)
		}
		return AgentResult{Output: output, CostUSD: resp.CostUSD, DurationS: resp.DurationS}, nil
	case "review":
		resp, err := agent.Review(ctx, agents.ReviewRequest{
			Prompt:       strings.TrimSpace(req.Prompt),
			SystemPrompt: strings.TrimSpace(req.SystemPrompt),
			Workdir:      strings.TrimSpace(req.Workdir),
			Model:        strings.TrimSpace(req.Model),
		})
		if err != nil {
			return AgentResult{}, err
		}
		return AgentResult{Output: strings.TrimSpace(resp.Output), CostUSD: resp.CostUSD, DurationS: resp.DurationS}, nil
	default:
		resp, err := agent.Execute(ctx, agents.ExecuteRequest{
			Prompt:       strings.TrimSpace(req.Prompt),
			SystemPrompt: strings.TrimSpace(req.SystemPrompt),
			Workdir:      strings.TrimSpace(req.Workdir),
			Model:        strings.TrimSpace(req.Model),
		})
		if err != nil {
			return AgentResult{}, err
		}
		return AgentResult{Output: strings.TrimSpace(resp.Output), CostUSD: resp.CostUSD, DurationS: resp.DurationS}, nil
	}
}
