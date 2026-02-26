package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GeminiCLI is a CLI-backed Agent implementation using the `gemini` CLI.
// Execute uses `gemini -p <prompt>` (one-shot, no PTY).
// Plan and Review use `gemini -p` with stdin + PTY (streaming).
type GeminiCLI struct {
	Binary string
	Runner CommandRunner
}

func NewGeminiCLI(binary string, runner CommandRunner) *GeminiCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "gemini"
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}
	return &GeminiCLI{Binary: binary, Runner: runner}
}

func (a *GeminiCLI) ID() ID { return Gemini }

func (a *GeminiCLI) Capabilities() Capabilities {
	return Capabilities{
		Transport:        TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      true,
		StructuredOutput: false,
	}
}

func (a *GeminiCLI) Plan(ctx context.Context, req PlanRequest) (PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY JSON with a dependency-aware execution plan for:\n\n" + objective
	if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	return a.run(ctx, req.Workdir, req.Model, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
}

func (a *GeminiCLI) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	start := time.Now()
	model := strings.TrimSpace(req.Model)

	args := []string{a.Binary, "-p"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, strings.TrimSpace(combinePrompt(req.SystemPrompt, req.Prompt)))

	result, err := a.Runner.Run(ctx, CommandSpec{
		Args:         args,
		Dir:          strings.TrimSpace(req.Workdir),
		UsePTY:       false,
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		WindowHint:   strings.TrimSpace(req.TaskLabel),
	})
	if err != nil {
		return ExecuteResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return ExecuteResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("gemini exit code %d: %s", result.ExitCode, tailText(result.Stderr, 20))
	}
	return ExecuteResponse{
		Output:    strings.TrimSpace(result.Stdout),
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *GeminiCLI) Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, combinePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return ReviewResponse{}, err
	}
	decision, reason := parseReview(resp.Output)
	return ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS, Strategy: resp.Strategy}, nil
}

func (a *GeminiCLI) run(ctx context.Context, workdir, model, prompt, runDir, outputPrefix, taskLabel string) (PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "-p"}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	result, err := a.Runner.Run(ctx, CommandSpec{
		Args:         args,
		Dir:          strings.TrimSpace(workdir),
		Stdin:        strings.TrimSpace(prompt),
		UsePTY:       true,
		RunDir:       strings.TrimSpace(runDir),
		OutputPrefix: strings.TrimSpace(outputPrefix),
		WindowHint:   strings.TrimSpace(taskLabel),
	})
	if err != nil {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("gemini exit code %d: %s", result.ExitCode, tailText(result.Stderr, 20))
	}
	return PlanResponse{Output: strings.TrimSpace(result.Stdout), Model: model, DurationS: time.Since(start).Seconds(), Strategy: result.Strategy}, nil
}
