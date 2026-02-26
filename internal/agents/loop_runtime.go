package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
	tmuxruntime "github.com/opus-domini/praetor/internal/runtime/tmux"
)

// RegistryRuntime routes loop requests through the central agents.Agent abstraction.
type RegistryRuntime struct {
	registry       *Registry
	sessionManager domain.SessionManager
}

// NewRegistryRuntime creates a RegistryRuntime wired with default agents and
// an exec-based command runner configured according to opts.
func NewRegistryRuntime(opts domain.RunnerOptions) *RegistryRuntime {
	var (
		runner CommandRunner
		sm     domain.SessionManager
	)

	switch opts.RunnerMode {
	case domain.RunnerTMUX:
		tmuxRunner := tmuxruntime.NewRunner(opts.TMUXSession)
		runner = NewProcessAdapterCommandRunner(tmuxRunner, "process")
		sm = tmuxRunner
	case domain.RunnerDirect:
		runner = NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{
			DisablePTY: true,
		})
	default:
		runner = NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{
			ForcePTY: opts.RunnerMode == domain.RunnerPTY,
		})
	}

	return &RegistryRuntime{
		registry: NewDefaultRegistry(DefaultOptions{
			CodexBin:    opts.CodexBin,
			ClaudeBin:   opts.ClaudeBin,
			GeminiBin:   opts.GeminiBin,
			OllamaURL:   opts.OllamaURL,
			OllamaModel: opts.OllamaModel,
			Runner:      runner,
		}),
		sessionManager: sm,
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
			RunDir:           strings.TrimSpace(req.RunDir),
			OutputPrefix:     strings.TrimSpace(req.OutputPrefix),
			TaskLabel:        strings.TrimSpace(req.TaskLabel),
		})
		if err != nil {
			return domain.AgentResult{}, err
		}
		output := strings.TrimSpace(resp.Output)
		if output == "" && len(resp.Manifest) > 0 {
			output = string(resp.Manifest)
		}
		return domain.AgentResult{Output: output, CostUSD: resp.CostUSD, DurationS: resp.DurationS, Strategy: domain.ExecutionStrategy(resp.Strategy)}, nil
	case "review":
		resp, err := agent.Review(ctx, ReviewRequest{
			Prompt:       strings.TrimSpace(req.Prompt),
			SystemPrompt: strings.TrimSpace(req.SystemPrompt),
			Workdir:      strings.TrimSpace(req.Workdir),
			Model:        strings.TrimSpace(req.Model),
			RunDir:       strings.TrimSpace(req.RunDir),
			OutputPrefix: strings.TrimSpace(req.OutputPrefix),
			TaskLabel:    strings.TrimSpace(req.TaskLabel),
		})
		if err != nil {
			return domain.AgentResult{}, err
		}
		return domain.AgentResult{Output: strings.TrimSpace(resp.Output), CostUSD: resp.CostUSD, DurationS: resp.DurationS, Strategy: domain.ExecutionStrategy(resp.Strategy)}, nil
	default:
		resp, err := agent.Execute(ctx, ExecuteRequest{
			Prompt:       strings.TrimSpace(req.Prompt),
			SystemPrompt: strings.TrimSpace(req.SystemPrompt),
			Workdir:      strings.TrimSpace(req.Workdir),
			Model:        strings.TrimSpace(req.Model),
			RunDir:       strings.TrimSpace(req.RunDir),
			OutputPrefix: strings.TrimSpace(req.OutputPrefix),
			TaskLabel:    strings.TrimSpace(req.TaskLabel),
		})
		if err != nil {
			return domain.AgentResult{}, err
		}
		return domain.AgentResult{Output: strings.TrimSpace(resp.Output), CostUSD: resp.CostUSD, DurationS: resp.DurationS, Strategy: domain.ExecutionStrategy(resp.Strategy)}, nil
	}
}

// EnsureSession implements domain.SessionManager when backed by a sessioned runner (tmux).
func (r *RegistryRuntime) EnsureSession() error {
	if r == nil || r.sessionManager == nil {
		return nil
	}
	return r.sessionManager.EnsureSession()
}

// Cleanup implements domain.SessionManager.
func (r *RegistryRuntime) Cleanup() {
	if r == nil || r.sessionManager == nil {
		return
	}
	r.sessionManager.Cleanup()
}

// SessionName implements domain.SessionManager.
func (r *RegistryRuntime) SessionName() string {
	if r == nil || r.sessionManager == nil {
		return ""
	}
	return r.sessionManager.SessionName()
}
