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

// GeminiCLI is a CLI-backed Agent implementation using the `gemini` CLI.
// Execute uses `gemini -p <prompt>` (one-shot, no PTY).
// Plan and Review use `gemini -p` with stdin + PTY (streaming).
type GeminiCLI struct {
	Binary string
	Runner runner.CommandRunner
}

func NewGeminiCLI(binary string, commandRunner runner.CommandRunner) *GeminiCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "gemini"
	}
	if commandRunner == nil {
		commandRunner = runner.NewExecCommandRunner()
	}
	return &GeminiCLI{Binary: binary, Runner: commandRunner}
}

func (a *GeminiCLI) ID() agent.ID { return agent.Gemini }

func (a *GeminiCLI) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      true,
		StructuredOutput: false,
	}
}

func (a *GeminiCLI) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return agent.PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY JSON with a dependency-aware execution plan for:\n\n" + objective
	if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	return a.run(ctx, req.Workdir, req.Model, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
}

func (a *GeminiCLI) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	if req.OneShot {
		return a.executeOneShot(ctx, req)
	}
	resp, err := a.run(ctx, req.Workdir, req.Model, agenttext.ComposePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.ExecuteResponse{}, err
	}
	return agent.ExecuteResponse{Output: resp.Output, Model: resp.Model, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *GeminiCLI) executeOneShot(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	start := time.Now()
	model := strings.TrimSpace(req.Model)

	args := []string{a.Binary, "-p"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, strings.TrimSpace(agenttext.ComposePrompt(req.SystemPrompt, req.Prompt)))

	result, err := a.Runner.Run(ctx, runner.CommandSpec{
		Args:         args,
		Dir:          strings.TrimSpace(req.Workdir),
		UsePTY:       false,
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		WindowHint:   strings.TrimSpace(req.TaskLabel),
	})
	if err != nil {
		return agent.ExecuteResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return agent.ExecuteResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("gemini exit code %d: %s", result.ExitCode, agenttext.TailText(result.Stderr, 20))
	}
	return agent.ExecuteResponse{
		Output:    strings.TrimSpace(result.Stdout),
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *GeminiCLI) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, agenttext.ComposePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.ReviewResponse{}, err
	}
	decision, reason := agenttext.ParseReview(resp.Output)
	return agent.ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *GeminiCLI) run(ctx context.Context, workdir, model, prompt, runDir, outputPrefix, taskLabel string) (agent.PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "-p"}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	result, err := a.Runner.Run(ctx, runner.CommandSpec{
		Args:         args,
		Dir:          strings.TrimSpace(workdir),
		Stdin:        strings.TrimSpace(prompt),
		UsePTY:       true,
		RunDir:       strings.TrimSpace(runDir),
		OutputPrefix: strings.TrimSpace(outputPrefix),
		WindowHint:   strings.TrimSpace(taskLabel),
	})
	if err != nil {
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("gemini exit code %d: %s", result.ExitCode, agenttext.TailText(result.Stderr, 20))
	}
	return agent.PlanResponse{Output: strings.TrimSpace(result.Stdout), Model: model, DurationS: time.Since(start).Seconds(), Strategy: result.Strategy}, nil
}
