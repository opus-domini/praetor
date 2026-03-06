package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

// CodexCLI is a CLI-backed Agent implementation using the `codex` CLI.
// Plan uses `codex exec --json` with `--output-schema` in a read-only sandbox.
// Execute (OneShot) uses `codex exec --json <prompt>` (no sandbox/approval flags).
// Review and pipeline execute use run() with full pipeline flags.
type CodexCLI struct {
	Binary string
	Runner runner.CommandRunner
}

func NewCodexCLI(binary string, commandRunner runner.CommandRunner) *CodexCLI {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "codex"
	}
	if commandRunner == nil {
		commandRunner = runner.NewExecCommandRunner()
	}
	return &CodexCLI{Binary: binary, Runner: commandRunner}
}

func (a *CodexCLI) ID() agent.ID { return agent.Codex }

func (a *CodexCLI) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Transport:        agent.TransportCLI,
		SupportsPlan:     true,
		SupportsExecute:  true,
		SupportsReview:   true,
		RequiresTTY:      false,
		StructuredOutput: true,
	}
}

func (a *CodexCLI) Plan(ctx context.Context, req agent.PlanRequest) (agent.PlanResponse, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return agent.PlanResponse{}, errors.New("objective is required")
	}
	prompt := "Return ONLY a valid JSON object execution plan for this objective:\n\n" + objective
	if req.PromptEngine != nil {
		if s, err := req.PromptEngine.Render("adapter.plan", adapterPlanData(objective, req.WorkspaceContext)); err == nil {
			prompt = s
		}
	} else if c := strings.TrimSpace(req.WorkspaceContext); c != "" {
		prompt = "Project context:\n" + c + "\n\n" + prompt
	}
	resp, err := a.plan(ctx, req.Workdir, req.Model, prompt, req.RunDir, req.OutputPrefix, req.TaskLabel)
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

func (a *CodexCLI) Execute(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	if req.OneShot {
		return a.executeOneShot(ctx, req)
	}
	resp, err := a.run(ctx, req.Workdir, req.Model, ComposePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
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

func (a *CodexCLI) executeOneShot(ctx context.Context, req agent.ExecuteRequest) (agent.ExecuteResponse, error) {
	start := time.Now()
	model := strings.TrimSpace(req.Model)

	args := []string{a.Binary, "exec", "--json"}
	if wd := strings.TrimSpace(req.Workdir); wd != "" {
		args = append(args, "--cd", wd)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, strings.TrimSpace(ComposePrompt(req.SystemPrompt, req.Prompt)))

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
		return agent.ExecuteResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("codex exit code %d: %s", result.ExitCode, TailText(result.Stderr, 20))
	}
	parsed := parseCodexOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	return agent.ExecuteResponse{
		Output:    parsed.Output,
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *CodexCLI) Review(ctx context.Context, req agent.ReviewRequest) (agent.ReviewResponse, error) {
	resp, err := a.run(ctx, req.Workdir, req.Model, ComposePrompt(req.SystemPrompt, req.Prompt), req.RunDir, req.OutputPrefix, req.TaskLabel)
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

func (a *CodexCLI) run(ctx context.Context, workdir, model, prompt, runDir, outputPrefix, taskLabel string) (agent.PlanResponse, error) {
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
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("codex exit code %d: %s", result.ExitCode, TailText(result.Stderr, 20))
	}

	parsed := parseCodexOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	return agent.PlanResponse{
		Output:    parsed.Output,
		Model:     model,
		DurationS: time.Since(start).Seconds(),
		Strategy:  result.Strategy,
	}, nil
}

func (a *CodexCLI) plan(ctx context.Context, workdir, model, prompt, runDir, outputPrefix, taskLabel string) (agent.PlanResponse, error) {
	start := time.Now()
	schemaPath, err := writePlannerOutputSchemaFile()
	if err != nil {
		return agent.PlanResponse{}, err
	}
	defer func() { _ = os.Remove(schemaPath) }()

	outputFile, err := os.CreateTemp("", "praetor-planner-output-*.json")
	if err != nil {
		return agent.PlanResponse{}, err
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer func() { _ = os.Remove(outputPath) }()

	args := []string{a.Binary, "exec", "--json",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--config", `approval_policy="never"`,
		"--ephemeral",
		"--output-schema", schemaPath,
		"--output-last-message", outputPath,
	}
	if strings.TrimSpace(workdir) != "" {
		args = append(args, "--cd", workdir)
	}
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
		return agent.PlanResponse{DurationS: time.Since(start).Seconds()}, fmt.Errorf("codex exit code %d: %s", result.ExitCode, TailText(result.Stderr, 20))
	}

	parsed := parseCodexOutput(result.Stdout)
	if parsed.Model != "" {
		model = parsed.Model
	}
	output, readErr := os.ReadFile(outputPath)
	if readErr == nil {
		if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
			parsed.Output = trimmed
		}
	}
	return agent.PlanResponse{
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
