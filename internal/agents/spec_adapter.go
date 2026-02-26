package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
)

// SpecAdapter bridges domain.AgentSpec + domain.ProcessRunner into agents.Agent.
type SpecAdapter struct {
	id     ID
	spec   domain.AgentSpec
	runner domain.ProcessRunner
}

func NewSpecAdapter(id ID, spec domain.AgentSpec, runner domain.ProcessRunner) (*SpecAdapter, error) {
	if spec == nil {
		return nil, fmt.Errorf("agent spec is required")
	}
	if runner == nil {
		return nil, fmt.Errorf("process runner is required")
	}
	return &SpecAdapter{id: id, spec: spec, runner: runner}, nil
}

func (a *SpecAdapter) ID() ID { return a.id }

func (a *SpecAdapter) Capabilities() Capabilities {
	return Capabilities{
		Transport:        TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: true,
	}
}

func (a *SpecAdapter) Plan(ctx context.Context, req PlanRequest) (PlanResponse, error) {
	start := time.Now()
	result, err := a.invoke(ctx, domain.AgentRequest{
		Role:         "plan",
		Prompt:       strings.TrimSpace(req.Objective),
		SystemPrompt: strings.TrimSpace(req.WorkspaceContext),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    strings.TrimSpace(req.TaskLabel),
	})
	if err != nil {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, err
	}
	return PlanResponse{
		Output:    strings.TrimSpace(result.Output),
		CostUSD:   result.CostUSD,
		DurationS: time.Since(start).Seconds(),
		Strategy:  string(result.Strategy),
	}, nil
}

func (a *SpecAdapter) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	start := time.Now()
	result, err := a.invoke(ctx, domain.AgentRequest{
		Role:         "execute",
		Prompt:       strings.TrimSpace(req.Prompt),
		SystemPrompt: strings.TrimSpace(req.SystemPrompt),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    strings.TrimSpace(req.TaskLabel),
	})
	if err != nil {
		return ExecuteResponse{DurationS: time.Since(start).Seconds()}, err
	}
	return ExecuteResponse{
		Output:    strings.TrimSpace(result.Output),
		CostUSD:   result.CostUSD,
		DurationS: time.Since(start).Seconds(),
		Strategy:  string(result.Strategy),
	}, nil
}

func (a *SpecAdapter) Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	start := time.Now()
	result, err := a.invoke(ctx, domain.AgentRequest{
		Role:         "review",
		Prompt:       strings.TrimSpace(req.Prompt),
		SystemPrompt: strings.TrimSpace(req.SystemPrompt),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    strings.TrimSpace(req.TaskLabel),
	})
	if err != nil {
		return ReviewResponse{DurationS: time.Since(start).Seconds()}, err
	}
	decision, reason := parseReview(result.Output)
	return ReviewResponse{
		Decision:  decision,
		Reason:    reason,
		Output:    strings.TrimSpace(result.Output),
		CostUSD:   result.CostUSD,
		DurationS: time.Since(start).Seconds(),
		Strategy:  string(result.Strategy),
	}, nil
}

func (a *SpecAdapter) invoke(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	if a == nil || a.spec == nil || a.runner == nil {
		return domain.AgentResult{}, fmt.Errorf("spec adapter is not initialized")
	}
	spec, err := a.spec.BuildCommand(req)
	if err != nil {
		return domain.AgentResult{}, err
	}
	spec.WindowHint = req.TaskLabel
	proc, err := a.runner.Run(ctx, spec, req.RunDir, req.OutputPrefix)
	if err != nil {
		return domain.AgentResult{}, err
	}
	if proc.ExitCode != 0 {
		return domain.AgentResult{}, fmt.Errorf("agent process failed with exit code %d: %s", proc.ExitCode, strings.TrimSpace(proc.Stderr))
	}
	output, cost, err := a.spec.ParseOutput(proc.Stdout)
	if err != nil {
		return domain.AgentResult{}, err
	}
	return domain.AgentResult{
		Output:   strings.TrimSpace(output),
		CostUSD:  cost,
		Strategy: domain.ExecutionStrategyProcess,
	}, nil
}
