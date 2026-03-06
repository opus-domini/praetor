package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/prompt"
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
	ID() domain.Agent
	Plan(ctx context.Context, req PlanRequest) (domain.Plan, error)
	Execute(ctx context.Context, req ExecuteRequest) (domain.AgentResult, error)
	Review(ctx context.Context, req ReviewRequest) (domain.ReviewDecision, domain.AgentResult, error)
}

// CognitiveOption configures optional behavior of a runtimeCognitiveAgent.
type CognitiveOption func(*runtimeCognitiveAgent)

// WithPromptEngine attaches a prompt.Engine to the cognitive agent.
func WithPromptEngine(e *prompt.Engine) CognitiveOption {
	return func(a *runtimeCognitiveAgent) {
		a.promptEngine = e
	}
}

// WithPlannerStrictMode toggles strict planner JSON parsing.
func WithPlannerStrictMode(strict bool) CognitiveOption {
	return func(a *runtimeCognitiveAgent) {
		a.plannerStrict = strict
	}
}

type runtimeCognitiveAgent struct {
	id            domain.Agent
	runtime       domain.AgentRuntime
	promptEngine  *prompt.Engine
	plannerStrict bool
}

// PlannerOutputError carries the raw planner output when parsing/validation fails.
type PlannerOutputError struct {
	Err       error
	RawOutput string
	Class     string
}

func (e *PlannerOutputError) Error() string {
	if e == nil || e.Err == nil {
		return "planner output error"
	}
	return e.Err.Error()
}

func (e *PlannerOutputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewCognitiveAgent creates a CognitiveAgent backed by the given runtime.
func NewCognitiveAgent(id domain.Agent, runtime domain.AgentRuntime, opts ...CognitiveOption) (CognitiveAgent, error) {
	id = domain.NormalizeAgent(id)
	if _, ok := domain.ValidExecutors[id]; !ok {
		return nil, fmt.Errorf("unsupported cognitive agent %q", id)
	}
	if runtime == nil {
		return nil, errors.New("runtime is required")
	}
	a := &runtimeCognitiveAgent{id: id, runtime: runtime, plannerStrict: true}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

func (a *runtimeCognitiveAgent) ID() domain.Agent {
	return a.id
}

func (a *runtimeCognitiveAgent) Plan(ctx context.Context, req PlanRequest) (domain.Plan, error) {
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		return domain.Plan{}, errors.New("objective is required")
	}
	systemPrompt := buildPlannerSystemPrompt(a.promptEngine, req.ProjectContext)
	userPrompt := buildPlannerTaskPrompt(a.promptEngine, objective)
	result, err := a.runtime.Run(ctx, domain.AgentRequest{
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
		return domain.Plan{}, err
	}
	payload, err := ExtractJSONObject(result.Output)
	if err != nil {
		return domain.Plan{}, &PlannerOutputError{
			Err:       fmt.Errorf("extract planner json payload: %w", err),
			RawOutput: result.Output,
			Class:     classifyPlannerParseError(err),
		}
	}
	var plan domain.Plan
	if a.plannerStrict {
		plan, err = domain.ParsePlanStrict([]byte(payload))
	} else {
		plan, err = domain.ParsePlanLenient([]byte(payload))
	}
	if err != nil {
		return domain.Plan{}, &PlannerOutputError{
			Err:       fmt.Errorf("decode planner output: %w", err),
			RawOutput: result.Output,
			Class:     classifyPlannerParseError(err),
		}
	}
	return plan, nil
}

func (a *runtimeCognitiveAgent) Execute(ctx context.Context, req ExecuteRequest) (domain.AgentResult, error) {
	return a.runtime.Run(ctx, domain.AgentRequest{
		Role:         "execute",
		Agent:        a.id,
		Prompt:       strings.TrimSpace(req.Prompt),
		SystemPrompt: BuildExecutorSystemPrompt(a.promptEngine, req.ProjectContext, nil, nil),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    "execute",
		CodexBin:     req.CodexBin,
		ClaudeBin:    req.ClaudeBin,
	})
}

func (a *runtimeCognitiveAgent) Review(ctx context.Context, req ReviewRequest) (domain.ReviewDecision, domain.AgentResult, error) {
	result, err := a.runtime.Run(ctx, domain.AgentRequest{
		Role:         "review",
		Agent:        a.id,
		Prompt:       strings.TrimSpace(req.Prompt),
		SystemPrompt: BuildReviewerSystemPrompt(a.promptEngine, req.ProjectContext, false),
		Model:        strings.TrimSpace(req.Model),
		Workdir:      strings.TrimSpace(req.Workdir),
		RunDir:       strings.TrimSpace(req.RunDir),
		OutputPrefix: strings.TrimSpace(req.OutputPrefix),
		TaskLabel:    "review",
		CodexBin:     req.CodexBin,
		ClaudeBin:    req.ClaudeBin,
	})
	if err != nil {
		return domain.ReviewDecision{}, domain.AgentResult{}, err
	}
	return domain.ParseReviewDecision(result.Output), result, nil
}

func buildPlannerSystemPrompt(engine *prompt.Engine, projectContext string) string {
	if engine != nil {
		if s, err := engine.Render("planner.system", prompt.PlannerSystemData{
			ProjectContext: strings.TrimSpace(projectContext),
		}); err == nil {
			return s
		}
	}
	var b strings.Builder
	if strings.TrimSpace(projectContext) != "" {
		fmt.Fprintf(&b, "## Project Context\n%s\n\n", strings.TrimSpace(projectContext))
	}
	b.WriteString(`You are a planning agent.
Return only valid JSON matching this schema:
{
  "name": "string",
  "summary": "string",
  "meta": {
    "source": "agent",
    "created_at": "RFC3339 timestamp"
  },
  "settings": {
    "agents": {
      "executor": {
        "agent": "claude|codex|copilot|gemini|kimi|opencode|openrouter|ollama|lmstudio",
        "model": "string optional"
      },
      "reviewer": {
        "agent": "claude|codex|copilot|gemini|kimi|opencode|openrouter|ollama|lmstudio|none",
        "model": "string optional"
      },
      "planner": {
        "agent": "claude|codex|copilot|gemini|kimi|opencode|openrouter|ollama|lmstudio",
        "model": "string optional"
      }
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "string",
      "depends_on": ["TASK-000"],
      "description": "string",
      "acceptance": ["string"]
    }
  ]
}

Rules:
- Create actionable, dependency-aware tasks.
- Use stable TASK-XXX ids in execution order.
- Keep each task atomic.
- Always include at least one acceptance item per task.
- Do not execute actions.
- Do not create files.
- Do not claim implementation is complete.
- Do not ask follow-up questions.
- If ambiguity remains, make a reasonable assumption and encode unresolved items in the JSON.
- Do not include markdown fences or commentary.
- Return JSON only.`)
	return b.String()
}

func buildPlannerTaskPrompt(engine *prompt.Engine, objective string) string {
	if engine != nil {
		if s, err := engine.Render("planner.task", prompt.PlannerTaskData{
			Objective: strings.TrimSpace(objective),
		}); err == nil {
			return s
		}
	}
	return fmt.Sprintf("Create an execution plan for this objective:\n\n%s", strings.TrimSpace(objective))
}

// ExtractJSONObject extracts the outermost JSON object from input text.
func ExtractJSONObject(input string) (string, error) {
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

func classifyPlannerParseError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "empty output"), strings.Contains(msg, "json object not found"):
		return "recoverable_format"
	case strings.Contains(msg, "unknown field"), strings.Contains(msg, "validation failed"), strings.Contains(msg, "decode plan file"):
		return "non_recoverable_schema"
	default:
		return "unknown"
	}
}
