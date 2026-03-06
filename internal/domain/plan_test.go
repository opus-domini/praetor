package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPlanAcceptsMinimal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "minimal.json")
	if err := os.WriteFile(planPath, []byte(`{
  "name": "minimal",
  "settings": {
    "agents": {
      "executor": {"agent": "codex"},
      "reviewer": {"agent": "claude"}
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "task",
      "acceptance": ["done"]
    }
  ]
}
`), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	plan, err := LoadPlan(planPath)
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if plan.Name != "minimal" {
		t.Fatalf("expected name=minimal, got %q", plan.Name)
	}
}

func TestLoadPlanRejectsUnknownFieldsOnSecondPass(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(planPath, []byte(`{
  "name": "unknown",
  "settings": {
    "agents": {
      "executor": {"agent": "codex"},
      "reviewer": {"agent": "claude"}
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "task",
      "acceptance": ["done"],
      "mystery": "field"
    }
  ]
}
`), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	_, err := LoadPlan(planPath)
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePlanRejectsInvalidTimeout(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Name: "timeout",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
			ExecutionPolicy: ExecutionPolicy{Timeout: "abc"},
		},
		Tasks: []Task{{ID: "TASK-001", Title: "task", Acceptance: []string{"done"}}},
	}
	if err := ValidatePlan(plan); err == nil {
		t.Fatal("expected timeout validation error")
	}
}

func TestValidatePlanRejectsDuplicateIDsAndMissingAcceptance(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Name: "bad",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Tasks: []Task{
			{ID: "TASK-001", Title: "a", Acceptance: []string{"ok"}},
			{ID: "TASK-001", Title: "b", Acceptance: nil},
		},
	}
	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(err.Error(), "duplicated id") {
		t.Fatalf("expected duplicated id error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "acceptance") {
		t.Fatalf("expected acceptance error, got: %v", err)
	}
}

func TestValidatePlanRejectsCyclesAndSelfDependency(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Name: "cycle",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Tasks: []Task{
			{ID: "TASK-001", Title: "a", DependsOn: []string{"TASK-001"}, Acceptance: []string{"ok"}},
			{ID: "TASK-002", Title: "b", DependsOn: []string{"TASK-003"}, Acceptance: []string{"ok"}},
			{ID: "TASK-003", Title: "c", DependsOn: []string{"TASK-002"}, Acceptance: []string{"ok"}},
		},
	}
	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("expected cycle/self-dependency error")
	}
	if !strings.Contains(err.Error(), "depends_on cannot reference itself") {
		t.Fatalf("expected self-dependency error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "contains cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestStateTasksFromPlanKeepsAcceptanceOnly(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Name: "state",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Tasks: []Task{{ID: "TASK-001", Title: "Task", Description: "desc", Acceptance: []string{"ok"}}},
	}
	tasks := StateTasksFromPlan(plan)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 state task, got %d", len(tasks))
	}
	if tasks[0].ID != "TASK-001" {
		t.Fatalf("expected id TASK-001, got %q", tasks[0].ID)
	}
	if len(tasks[0].Acceptance) != 1 || tasks[0].Acceptance[0] != "ok" {
		t.Fatalf("expected acceptance to be copied, got %+v", tasks[0].Acceptance)
	}
}

func TestNewPlanFileGeneratesValidPlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := NewPlanFile("my-plan", dir)
	if err != nil {
		t.Fatalf("new plan file: %v", err)
	}
	_, err = LoadPlan(path)
	if err != nil {
		t.Fatalf("load generated plan: %v", err)
	}
}

func TestLoadPlanAcceptsFullFeatures(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "full.json")
	if err := os.WriteFile(planPath, []byte(`{
  "name": "full-plan",
  "cognitive": {
    "assumptions": ["REST API"],
    "open_questions": ["auth method"],
    "failure_modes": ["rollback via snapshot"],
    "decisions": ["use interfaces"]
  },
  "settings": {
    "agents": {
      "executor": {"agent": "codex"},
      "reviewer": {"agent": "claude"}
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "task",
      "acceptance": ["done"],
      "constraints": {
        "allowed_tools": ["read", "edit"],
        "timeout": "30m"
      },
      "agents": {
        "executor": "claude",
        "reviewer": "none"
	      }
	    }
	  ]
	}
`), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	plan, err := LoadPlan(planPath)
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if plan.Cognitive == nil {
		t.Fatal("expected cognitive to be set")
	}
	if len(plan.Cognitive.Assumptions) != 1 || plan.Cognitive.Assumptions[0] != "REST API" {
		t.Fatalf("expected assumptions=[REST API], got %+v", plan.Cognitive.Assumptions)
	}
	if plan.Tasks[0].Constraints == nil {
		t.Fatal("expected constraints to be set")
	}
	if len(plan.Tasks[0].Constraints.AllowedTools) != 2 {
		t.Fatalf("expected 2 allowed tools, got %d", len(plan.Tasks[0].Constraints.AllowedTools))
	}
	if plan.Tasks[0].Agents == nil {
		t.Fatal("expected agents to be set")
	}
	if plan.Tasks[0].Agents.Executor != "claude" {
		t.Fatalf("expected per-task executor=claude, got %q", plan.Tasks[0].Agents.Executor)
	}
}

func TestLoadPlanAcceptsPromptBudgetAndCostPolicy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(planPath, []byte(`{
  "name": "policy-plan",
  "settings": {
    "agents": {
      "executor": {"agent": "codex"},
      "reviewer": {"agent": "claude"}
    },
    "execution_policy": {
      "prompt_budget": {
        "executor_chars": 120000,
        "reviewer_chars": 80000
      },
      "cost": {
        "plan_limit_cents": 1500,
        "task_limit_cents": 500,
        "warn_threshold": 0.8,
        "enforce": true
      }
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "task",
      "acceptance": ["done"]
    }
  ]
}
`), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	plan, err := LoadPlan(planPath)
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	if plan.Settings.ExecutionPolicy.PromptBudget.ExecutorChars != 120000 {
		t.Fatalf("executor prompt chars = %d, want 120000", plan.Settings.ExecutionPolicy.PromptBudget.ExecutorChars)
	}
	if plan.Settings.ExecutionPolicy.PromptBudget.ReviewerChars != 80000 {
		t.Fatalf("reviewer prompt chars = %d, want 80000", plan.Settings.ExecutionPolicy.PromptBudget.ReviewerChars)
	}
	if plan.Settings.ExecutionPolicy.Cost.PlanLimitCents != 1500 {
		t.Fatalf("plan limit cents = %d, want 1500", plan.Settings.ExecutionPolicy.Cost.PlanLimitCents)
	}
	if plan.Settings.ExecutionPolicy.Cost.TaskLimitCents != 500 {
		t.Fatalf("task limit cents = %d, want 500", plan.Settings.ExecutionPolicy.Cost.TaskLimitCents)
	}
	if plan.Settings.ExecutionPolicy.Cost.WarnThreshold != 0.8 {
		t.Fatalf("warn threshold = %.2f, want 0.80", plan.Settings.ExecutionPolicy.Cost.WarnThreshold)
	}
	if plan.Settings.ExecutionPolicy.Cost.Enforce == nil || !*plan.Settings.ExecutionPolicy.Cost.Enforce {
		t.Fatal("expected enforce=true")
	}
}

func TestValidatePlanRejectsInvalidTaskConstraintTimeout(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Name: "bad-timeout",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Tasks: []Task{{
			ID:          "TASK-001",
			Title:       "task",
			Acceptance:  []string{"done"},
			Constraints: &TaskConstraints{Timeout: "invalid"},
		}},
	}
	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("expected constraint timeout validation error")
	}
	if !strings.Contains(err.Error(), "constraints.timeout") {
		t.Fatalf("expected constraints.timeout error, got: %v", err)
	}
}

func TestValidatePlanRejectsInvalidCostWarnThreshold(t *testing.T) {
	t.Parallel()

	enforce := true
	plan := Plan{
		Name: "bad-cost-threshold",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
			ExecutionPolicy: ExecutionPolicy{
				Cost: CostPolicy{
					PlanLimitCents: 1000,
					WarnThreshold:  1.2,
					Enforce:        &enforce,
				},
			},
		},
		Tasks: []Task{{ID: "TASK-001", Title: "task", Acceptance: []string{"done"}}},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("expected invalid cost threshold error")
	}
	if !strings.Contains(err.Error(), "settings.execution_policy.cost.warn_threshold") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParsePlanStrictRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := ParsePlanStrict([]byte(`{
  "name": "strict-plan",
  "settings": {
    "agents": {
      "executor": {"agent":"codex"},
      "reviewer": {"agent":"claude"}
    }
  },
  "tasks": [
    {"id":"TASK-001","title":"x","acceptance":["ok"],"mystery":"field"}
  ]
}`))
	if err == nil {
		t.Fatal("expected strict parse error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePlanRejectsUnsupportedQualityCommandGate(t *testing.T) {
	t.Parallel()

	plan := Plan{
		Name: "bad-gate-cmd",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Quality: PlanQuality{
			Commands: map[string]string{
				"custom": "echo custom",
			},
		},
		Tasks: []Task{
			{ID: "TASK-001", Title: "task", Acceptance: []string{"done"}},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "quality.commands contains unsupported gate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePlanRejectsInvalidPerTaskAgent(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Name: "bad-agent",
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Tasks: []Task{{
			ID:         "TASK-001",
			Title:      "task",
			Acceptance: []string{"done"},
			Agents:     &TaskAgents{Executor: "invalid-agent"},
		}},
	}
	err := ValidatePlan(plan)
	if err == nil {
		t.Fatal("expected per-task agent validation error")
	}
	if !strings.Contains(err.Error(), "agents.executor") {
		t.Fatalf("expected agents.executor error, got: %v", err)
	}
}
