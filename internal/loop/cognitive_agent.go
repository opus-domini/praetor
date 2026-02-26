package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PlanRequest is the macro-planning request contract.
type PlanRequest struct {
	Objective      string
	ProjectContext string
	Workdir        string
	Model          string
	CodexBin       string
	ClaudeBin      string
}

// ExecuteRequest is the atomic execution request contract.
type ExecuteRequest struct {
	Prompt         string
	ProjectContext string
	Workdir        string
	RunDir         string
	OutputPrefix   string
	Model          string
	CodexBin       string
	ClaudeBin      string
}

// ReviewRequest is the isolated review request contract.
type ReviewRequest struct {
	Prompt         string
	ProjectContext string
	Workdir        string
	RunDir         string
	OutputPrefix   string
	Model          string
	CodexBin       string
	ClaudeBin      string
}

// CognitiveAgent is the central polymorphic contract for Plan/Execute/Review.
type CognitiveAgent interface {
	ID() Agent
	Plan(ctx context.Context, req PlanRequest) (Plan, error)
	Execute(ctx context.Context, req ExecuteRequest) (AgentResult, error)
	Review(ctx context.Context, req ReviewRequest) (ReviewDecision, AgentResult, error)
}

type runtimeCognitiveAgent struct {
	id      Agent
	runtime AgentRuntime
}

func NewCognitiveAgent(id Agent, runtime AgentRuntime) (CognitiveAgent, error) {
	id = normalizeAgent(id)
	if _, ok := validExecutors[id]; !ok {
		return nil, fmt.Errorf("unsupported cognitive agent %q", id)
	}
	if runtime == nil {
		return nil, errors.New("runtime is required")
	}
	return &runtimeCognitiveAgent{id: id, runtime: runtime}, nil
}

func (a *runtimeCognitiveAgent) ID() Agent {
	return a.id
}

func (a *runtimeCognitiveAgent) Plan(ctx context.Context, req PlanRequest) (Plan, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return Plan{}, errors.New("objective is required")
	}
	systemPrompt := buildPlannerSystemPrompt(req.ProjectContext)
	userPrompt := buildPlannerTaskPrompt(objective)
	result, err := a.runtime.Run(ctx, AgentRequest{
		Role:         "plan",
		Agent:        a.id,
		Prompt:       userPrompt,
		SystemPrompt: systemPrompt,
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       "",
		OutputPrefix: "planner",
		TaskLabel:    "planner",
		CodexBin:     req.CodexBin,
		ClaudeBin:    req.ClaudeBin,
	})
	if err != nil {
		return Plan{}, err
	}
	payload, err := extractJSONObject(result.Output)
	if err != nil {
		return Plan{}, fmt.Errorf("extract planner json payload: %w", err)
	}
	plan := Plan{}
	if err := json.Unmarshal([]byte(payload), &plan); err != nil {
		return Plan{}, fmt.Errorf("decode planner output: %w", err)
	}
	if strings.TrimSpace(plan.Title) == "" {
		plan.Title = "generated plan"
	}
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, fmt.Errorf("planner generated invalid plan: %w", err)
	}
	return plan, nil
}

func (a *runtimeCognitiveAgent) Execute(ctx context.Context, req ExecuteRequest) (AgentResult, error) {
	return a.runtime.Run(ctx, AgentRequest{
		Role:         "execute",
		Agent:        a.id,
		Prompt:       strings.TrimSpace(req.Prompt),
		SystemPrompt: buildExecutorSystemPrompt(req.ProjectContext),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    "execute",
		CodexBin:     req.CodexBin,
		ClaudeBin:    req.ClaudeBin,
	})
}

func (a *runtimeCognitiveAgent) Review(ctx context.Context, req ReviewRequest) (ReviewDecision, AgentResult, error) {
	result, err := a.runtime.Run(ctx, AgentRequest{
		Role:         "review",
		Agent:        a.id,
		Prompt:       strings.TrimSpace(req.Prompt),
		SystemPrompt: buildReviewerSystemPrompt(req.ProjectContext),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    "review",
		CodexBin:     req.CodexBin,
		ClaudeBin:    req.ClaudeBin,
	})
	if err != nil {
		return ReviewDecision{}, AgentResult{}, err
	}
	return ParseReviewDecision(result.Output), result, nil
}

func buildPlannerSystemPrompt(projectContext string) string {
	var b strings.Builder
	if strings.TrimSpace(projectContext) != "" {
		fmt.Fprintf(&b, "## Project Context\n%s\n\n", strings.TrimSpace(projectContext))
	}
	b.WriteString(`You are a planning agent.
Return only valid JSON matching this schema:
{
  "$schema": "../schemas/loop-plan.schema.json",
  "title": "string",
  "tasks": [
    {
      "id": "TASK-001",
      "title": "string",
      "depends_on": ["TASK-000"],
      "executor": "codex|claude",
      "reviewer": "codex|claude|none",
      "description": "string",
      "criteria": "string"
    }
  ]
}

Rules:
- Create actionable, dependency-aware tasks.
- Use stable TASK-XXX ids in execution order.
- Keep each task atomic.
- Return JSON only.`)
	return b.String()
}

func buildPlannerTaskPrompt(objective string) string {
	return fmt.Sprintf("Create an execution plan for this objective:\n\n%s", strings.TrimSpace(objective))
}

func extractJSONObject(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty output")
	}
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start < 0 || end < 0 || end <= start {
		return "", errors.New("json object not found")
	}
	return strings.TrimSpace(input[start : end+1]), nil
}
