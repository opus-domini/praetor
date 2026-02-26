package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CodexCLI is a CLI-backed Agent implementation using `codex exec --json`.
type CodexCLI struct {
	Binary string
	Runner CommandRunner
}

func NewCodexCLI(binary string, runner CommandRunner) *CodexCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "codex"
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}
	return &CodexCLI{Binary: binary, Runner: runner}
}

func (a *CodexCLI) ID() ID { return Codex }

func (a *CodexCLI) Capabilities() Capabilities {
	return Capabilities{
		Transport:        TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: true,
	}
}

func (a *CodexCLI) Plan(ctx context.Context, req PlanRequest) (PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY a valid JSON object execution plan for this objective:\n\n" + objective
	if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	resp, err := a.run(ctx, req.Workdir, req.Model, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return PlanResponse{}, err
	}
	obj, err := extractJSONObject(resp.Output)
	if err == nil {
		resp.Manifest = json.RawMessage(obj)
	}
	return resp, nil
}

func (a *CodexCLI) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, combinePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return ExecuteResponse{}, err
	}
	return ExecuteResponse{
		Output:    resp.Output,
		Model:     resp.Model,
		CostUSD:   resp.CostUSD,
		DurationS: resp.DurationS,
		Strategy:  resp.Strategy,
	}, nil
}

func (a *CodexCLI) Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, combinePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return ReviewResponse{}, err
	}
	decision, reason := parseReview(resp.Output)
	return ReviewResponse{
		Decision:  decision,
		Reason:    reason,
		Output:    resp.Output,
		CostUSD:   resp.CostUSD,
		DurationS: resp.DurationS,
		Strategy:  resp.Strategy,
	}, nil
}

func (a *CodexCLI) run(ctx context.Context, workdir, model, prompt, runDir, outputPrefix, taskLabel string) (PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "exec", "--json",
		"--sandbox", "workspace-write",
		"--skip-git-repo-check",
		"--config", `approval_policy="never"`,
	}
	if strings.TrimSpace(workdir) != "" {
		args = append(args, "--cd", workdir)
	}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, strings.TrimSpace(prompt))

	result, err := a.Runner.Run(ctx, CommandSpec{
		Args:         args,
		Dir:          strings.TrimSpace(workdir),
		UsePTY:       false,
		RunDir:       strings.TrimSpace(runDir),
		OutputPrefix: strings.TrimSpace(outputPrefix),
		WindowHint:   strings.TrimSpace(taskLabel),
	})
	if err != nil {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, err
	}
	if result.ExitCode != 0 {
		return PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("codex exit code %d: %s", result.ExitCode, tailText(result.Stderr, 20))
	}

	parsed := parseCodexOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	return PlanResponse{
		Output:    parsed.Output,
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

type codexStreamEvent struct {
	Type  string `json:"type"`
	Model string `json:"model,omitempty"`
	Item  struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item,omitempty"`
}

type codexParsed struct {
	Output string
	Model  string
}

func parseCodexOutput(stdout string) codexParsed {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return codexParsed{}
	}
	var parts []string
	var model string
	isJSONL := false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		event := codexStreamEvent{}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "" {
			continue
		}
		isJSONL = true
		if event.Model != "" {
			model = event.Model
		}
		if event.Type == "item.completed" && event.Item.Type == "agent_message" {
			if text := strings.TrimSpace(event.Item.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	if isJSONL && len(parts) > 0 {
		return codexParsed{Output: strings.Join(parts, "\n"), Model: model}
	}
	return codexParsed{Output: stdout, Model: model}
}

func tailText(input string, maxLines int) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "no stderr output"
	}
	lines := strings.Split(input, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, " | ")
}
