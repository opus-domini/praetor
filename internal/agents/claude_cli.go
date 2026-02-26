package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ClaudeCLI is a CLI-backed Agent implementation using the `claude` CLI.
// Execute uses `claude -p --output-format json` (one-shot, no PTY).
// Plan and Review use `claude -p --output-format stream-json` (streaming, PTY).
type ClaudeCLI struct {
	Binary string
	Runner CommandRunner
}

func NewClaudeCLI(binary string, runner CommandRunner) *ClaudeCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "claude"
	}
	if runner == nil {
		runner = NewExecCommandRunner()
	}
	return &ClaudeCLI{Binary: binary, Runner: runner}
}

func (a *ClaudeCLI) ID() ID { return Claude }

func (a *ClaudeCLI) Capabilities() Capabilities {
	return Capabilities{
		Transport:        TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      true,
		StructuredOutput: true,
	}
}

func (a *ClaudeCLI) Plan(ctx context.Context, req PlanRequest) (PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return PlanResponse{}, errors.New("objective is required")
	}
	systemPrompt := "You are a planning agent. Return only valid JSON."
	if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		systemPrompt = "Project context:\n" + c + "\n\n" + systemPrompt
	}
	prompt := "Create a dependency-aware execution plan for:\n\n" + objective
	resp, err := a.run(ctx, req.Workdir, req.Model, systemPrompt, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return PlanResponse{}, err
	}
	obj, err := extractJSONObject(resp.Output)
	if err == nil {
		resp.Manifest = json.RawMessage(obj)
	}
	return resp, nil
}

func (a *ClaudeCLI) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	if req.OneShot {
		return a.executeOneShot(ctx, req)
	}
	resp, err := a.run(ctx, req.Workdir, req.Model, req.SystemPrompt, req.Prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
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

func (a *ClaudeCLI) executeOneShot(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	start := time.Now()
	model := strings.TrimSpace(req.Model)

	args := []string{a.Binary, "-p", "--output-format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	if sp := strings.TrimSpace(req.SystemPrompt); sp != "" {
		args = append(args, "--append-system-prompt", sp)
	}
	args = append(args, strings.TrimSpace(req.Prompt))

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
		return ExecuteResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("claude exit code %d: %s", result.ExitCode, tailText(result.Stderr, 20))
	}
	parsed := parseClaudeOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	return ExecuteResponse{
		Output:    parsed.Output,
		Model:     model,
		CostUSD:   parsed.CostUSD,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *ClaudeCLI) Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, req.SystemPrompt, req.Prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
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

func (a *ClaudeCLI) run(ctx context.Context, workdir, model, systemPrompt, prompt, runDir, outputPrefix, taskLabel string) (PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "-p",
		"--dangerously-skip-permissions",
		"--no-session-persistence",
		"--verbose",
		"--output-format", "stream-json",
	}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
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
		return PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("claude exit code %d: %s", result.ExitCode, tailText(result.Stderr, 20))
	}
	output, cost := parseStreamOutput(result.Stdout)
	return PlanResponse{
		Output:    output,
		Model:     model,
		CostUSD:   cost,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

// parseClaudeOutput parses the single JSON object from `claude -p --output-format json`.
func parseClaudeOutput(stdout string) claudeParsed {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return claudeParsed{}
	}
	var resp struct {
		Result  string  `json:"result"`
		Model   string  `json:"model"`
		CostUSD float64 `json:"cost_usd"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return claudeParsed{Output: stdout}
	}
	output := strings.TrimSpace(resp.Result)
	if output == "" {
		output = stdout
	}
	return claudeParsed{
		Output:  output,
		Model:   strings.TrimSpace(resp.Model),
		CostUSD: resp.CostUSD,
	}
}

type claudeParsed struct {
	Output  string
	Model   string
	CostUSD float64
}

// parseStreamOutput parses JSONL from `claude -p --output-format stream-json`.
// Used by run() for the plan pipeline.
func parseStreamOutput(stdout string) (string, float64) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", 0
	}
	type streamEvent struct {
		Type    string  `json:"type"`
		Result  string  `json:"result,omitempty"`
		CostUSD float64 `json:"cost_usd,omitempty"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	var lastResult *streamEvent
	var assistantText []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		event := streamEvent{}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "result" {
			e := event
			lastResult = &e
		}
		if event.Type == "assistant" {
			for _, c := range event.Message.Content {
				if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
					assistantText = append(assistantText, strings.TrimSpace(c.Text))
				}
			}
		}
	}
	if lastResult != nil {
		result := strings.TrimSpace(lastResult.Result)
		if result != "" {
			return result, lastResult.CostUSD
		}
		if len(assistantText) > 0 {
			return strings.Join(assistantText, "\n"), lastResult.CostUSD
		}
		return "", lastResult.CostUSD
	}
	if len(assistantText) > 0 {
		return strings.Join(assistantText, "\n"), 0
	}
	return stdout, 0
}
