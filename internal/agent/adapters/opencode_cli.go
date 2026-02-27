package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
	agenttext "github.com/opus-domini/praetor/internal/agent/text"
)

// OpenCodeCLI is a CLI-backed Agent implementation using `opencode run --quiet`.
type OpenCodeCLI struct {
	Binary string
	Runner runner.CommandRunner
}

func NewOpenCodeCLI(binary string, commandRunner runner.CommandRunner) *OpenCodeCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "opencode"
	}
	if commandRunner == nil {
		commandRunner = runner.NewExecCommandRunner()
	}
	return &OpenCodeCLI{Binary: binary, Runner: commandRunner}
}

func (a *OpenCodeCLI) ID() agent.ID { return agent.OpenCode }

func (a *OpenCodeCLI) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: false,
	}
}

func (a *OpenCodeCLI) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return agent.PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY JSON with a dependency-aware execution plan for:\n\n" + objective
	if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	resp, err := a.run(ctx, req.Workdir, req.Model, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.PlanResponse{}, err
	}
	obj, err := agenttext.ExtractJSONObject(resp.Output)
	if err == nil {
		resp.Manifest = []byte(obj)
	}
	return resp, nil
}

func (a *OpenCodeCLI) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, agenttext.ComposePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.ExecuteResponse{}, err
	}
	return agent.ExecuteResponse{Output: resp.Output, Model: resp.Model, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *OpenCodeCLI) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, agenttext.ComposePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.ReviewResponse{}, err
	}
	decision, reason := agenttext.ParseReview(resp.Output)
	return agent.ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *OpenCodeCLI) run(ctx context.Context, workdir, model, prompt, runDir, outputPrefix, taskLabel string) (agent.PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "run", "--quiet"}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, strings.TrimSpace(prompt))

	result, err := a.Runner.Run(ctx, runner.CommandSpec{
		Args:         args,
		Dir:          strings.TrimSpace(workdir),
		UsePTY:       false,
		RunDir:       strings.TrimSpace(runDir),
		OutputPrefix: strings.TrimSpace(outputPrefix),
		WindowHint:   strings.TrimSpace(taskLabel),
	})
	if err != nil {
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("opencode exit code %d: %s", result.ExitCode, agenttext.TailText(result.Stderr, 20))
	}
	return agent.PlanResponse{
		Output:    strings.TrimSpace(result.Stdout),
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}
