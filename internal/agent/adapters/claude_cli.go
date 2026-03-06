package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

// ClaudeCLI is a CLI-backed Agent implementation using the `claude` CLI.
// Plan uses `claude -p --output-format json` with `--json-schema`.
// Execute uses `claude -p --output-format json` for one-shot and `stream-json` in the pipeline.
// Review uses `claude -p --output-format stream-json` (streaming, PTY).
type ClaudeCLI struct {
	Binary string
	Runner runner.CommandRunner
}

func NewClaudeCLI(binary string, commandRunner runner.CommandRunner) *ClaudeCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "claude"
	}
	if commandRunner == nil {
		commandRunner = runner.NewExecCommandRunner()
	}
	return &ClaudeCLI{Binary: binary, Runner: commandRunner}
}

func (a *ClaudeCLI) ID() agent.ID { return agent.Claude }

func (a *ClaudeCLI) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      true,
		StructuredOutput: true,
	}
}

func (a *ClaudeCLI) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return agent.PlanResponse{}, errors.New("objective is required")
	}
	systemPrompt := "You are a planning agent. Return only one valid JSON object matching the requested schema. Never ask follow-up questions. If context is ambiguous, make reasonable assumptions and encode unresolved items in the JSON instead of asking."
	prompt := "Create a dependency-aware execution plan for:\n\n" + objective
	if req.PromptEngine != nil {
		if s, err := req.PromptEngine.Render("adapter.plan.claude", adapterPlanData(objective, req.WorkspaceContext)); err == nil {
			systemPrompt = s
		}
		if s, err := req.PromptEngine.Render("adapter.plan", adapterPlanData(objective, req.WorkspaceContext)); err == nil {
			prompt = s
		}
	} else {
		if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
			systemPrompt = "Project context:\n" + c + "\n\n" + systemPrompt
		}
	}
	resp, err := a.plan(ctx, req.Workdir, req.Model, systemPrompt, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.PlanResponse{}, err
	}
	if manifest := plannerManifest(resp.Output); len(manifest) > 0 {
		resp.Manifest = manifest
		return resp, nil
	}
	obj, err := ExtractJSONObject(resp.Output)
	if err == nil && json.Valid([]byte(obj)) {
		resp.Manifest = json.RawMessage(obj)
	}
	return resp, nil
}

func (a *ClaudeCLI) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	if req.OneShot {
		return a.executeOneShot(ctx, req)
	}
	resp, err := a.run(ctx, req.Workdir, req.Model, req.SystemPrompt, req.Prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.ExecuteResponse{}, err
	}
	return agent.ExecuteResponse{
		Output:    resp.Output,
		Model:     resp.Model,
		CostUSD:   resp.CostUSD,
		DurationS: resp.DurationS,
		Strategy:  resp.Strategy,
	}, nil
}

func (a *ClaudeCLI) executeOneShot(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
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
		return agent.ExecuteResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("claude exit code %d: %s", result.ExitCode, TailText(result.Stderr, 20))
	}
	parsed := parseClaudeOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	return agent.ExecuteResponse{
		Output:    parsed.Output,
		Model:     model,
		CostUSD:   parsed.CostUSD,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *ClaudeCLI) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, req.SystemPrompt, req.Prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
	if err != nil {
		return agent.ReviewResponse{}, err
	}
	decision, reason := ParseReview(resp.Output)
	return agent.ReviewResponse{
		Decision:  decision,
		Reason:    reason,
		Output:    resp.Output,
		CostUSD:   resp.CostUSD,
		DurationS: resp.DurationS,
		Strategy:  resp.Strategy,
	}, nil
}

func (a *ClaudeCLI) run(ctx context.Context, workdir, model, systemPrompt, prompt, runDir, outputPrefix, taskLabel string) (agent.PlanResponse, error) {
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
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("claude exit code %d: %s", result.ExitCode, TailText(result.Stderr, 20))
	}
	output, cost := parseStreamOutput(result.Stdout)
	return agent.PlanResponse{
		Output:    output,
		Model:     model,
		CostUSD:   cost,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *ClaudeCLI) plan(ctx context.Context, workdir, model, systemPrompt, prompt, runDir, outputPrefix, taskLabel string) (agent.PlanResponse, error) {
	start := time.Now()
	args := []string{a.Binary, "-p",
		"--no-session-persistence",
		"--verbose",
		"--output-format", "json",
		"--tools", "",
		"--disable-slash-commands",
		"--json-schema", plannerOutputSchema(),
	}
	if model = strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
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
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("claude exit code %d: %s", result.ExitCode, TailText(result.Stderr, 20))
	}
	parsed := parseClaudeOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	return agent.PlanResponse{
		Output:    parsed.Output,
		Model:     model,
		CostUSD:   parsed.CostUSD,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

// parseClaudeOutput parses JSON emitted by `claude -p --output-format json`.
func parseClaudeOutput(stdout string) claudeParsed {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return claudeParsed{}
	}
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	}
	type messageEnvelope struct {
		Model   string         `json:"model,omitempty"`
		Content []contentBlock `json:"content,omitempty"`
	}
	type resultEnvelope struct {
		Type             string          `json:"type,omitempty"`
		Result           string          `json:"result,omitempty"`
		Model            string          `json:"model,omitempty"`
		CostUSD          float64         `json:"cost_usd,omitempty"`
		StructuredOutput json.RawMessage `json:"structured_output,omitempty"`
		Message          messageEnvelope `json:"message,omitempty"`
	}

	parseEnvelope := func(resp resultEnvelope, fallback string) claudeParsed {
		output := ""
		if len(resp.StructuredOutput) > 0 && json.Valid(resp.StructuredOutput) {
			output = string(resp.StructuredOutput)
		}
		if output == "" {
			output = strings.TrimSpace(resp.Result)
		}
		if output == "" && len(resp.Message.Content) > 0 {
			var parts []string
			for _, block := range resp.Message.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					parts = append(parts, strings.TrimSpace(block.Text))
				}
			}
			if len(parts) > 0 {
				output = strings.Join(parts, "\n")
			}
		}
		if output == "" {
			output = fallback
		}
		model := strings.TrimSpace(resp.Model)
		if model == "" {
			model = strings.TrimSpace(resp.Message.Model)
		}
		return claudeParsed{
			Output:  output,
			Model:   model,
			CostUSD: resp.CostUSD,
		}
	}

	if strings.HasPrefix(stdout, "[") {
		var events []resultEnvelope
		if err := json.Unmarshal([]byte(stdout), &events); err == nil {
			var (
				lastResult    resultEnvelope
				hasResult     bool
				assistantText []string
			)
			for _, event := range events {
				if event.Type == "result" {
					lastResult = event
					hasResult = true
					continue
				}
				if event.Type == "assistant" {
					for _, block := range event.Message.Content {
						if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
							assistantText = append(assistantText, strings.TrimSpace(block.Text))
						}
					}
				}
			}
			if hasResult {
				parsed := parseEnvelope(lastResult, stdout)
				if strings.TrimSpace(parsed.Output) == stdout && len(assistantText) > 0 {
					parsed.Output = strings.Join(assistantText, "\n")
				}
				return parsed
			}
			if len(assistantText) > 0 {
				return claudeParsed{Output: strings.Join(assistantText, "\n")}
			}
		}
		return claudeParsed{Output: stdout}
	}

	var resp resultEnvelope
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return claudeParsed{Output: stdout}
	}
	return parseEnvelope(resp, stdout)
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
