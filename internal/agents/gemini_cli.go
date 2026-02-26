package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// GeminiCLI is a CLI-backed Agent implementation.
// It assumes a `gemini -p` style prompt API.
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
	return a.run(ctx, req.Workdir, req.Model, prompt)
}

func (a *GeminiCLI) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, combinePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return ExecuteResponse{}, err
	}
	return ExecuteResponse{Output: resp.Output, DurationS: resp.DurationS}, nil
}

func (a *GeminiCLI) Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, combinePrompt(req.SystemPrompt, req.Prompt))
	if err != nil {
		return ReviewResponse{}, err
	}
	decision, reason := parseReview(resp.Output)
	return ReviewResponse{Decision: decision, Reason: reason, Output: resp.Output, DurationS: resp.DurationS}, nil
}

func (a *GeminiCLI) run(ctx context.Context, workdir, model, prompt string) (PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "-p"}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	result, err := a.Runner.Run(ctx, CommandSpec{
		Args:   args,
		Dir:    strings.TrimSpace(workdir),
		Stdin:  strings.TrimSpace(prompt),
		UsePTY: true,
	})
	if err != nil {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("gemini exit code %d: %s", result.ExitCode, tailText(result.Stderr, 20))
	}
	return PlanResponse{Output: strings.TrimSpace(result.Stdout), DurationS: time.Since(start).Seconds()}, nil
}
