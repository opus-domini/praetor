package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPlanAcceptsMinimalV1(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "minimal.json")
	if err := os.WriteFile(planPath, []byte(`{
  "schema_version": 1,
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
	if plan.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1, got %d", plan.SchemaVersion)
	}
	if plan.Name != "minimal" {
		t.Fatalf("expected name=minimal, got %q", plan.Name)
	}
}

func TestLoadPlanRejectsLegacyFieldsWithGuidance(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(planPath, []byte(`{
  "schema_version": 1,
  "name": "legacy",
  "title": "legacy",
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
      "criteria": "old",
      "executor": "codex",
      "acceptance": ["done"]
    }
  ]
}
`), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	_, err := LoadPlan(planPath)
	if err == nil {
		t.Fatal("expected legacy field error")
	}
	msg := err.Error()
	for _, want := range []string{
		"Field 'title' is no longer supported",
		"criteria",
		"Per-task agent fields are no longer supported",
		"Recreate with: praetor plan create",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected error to contain %q, got: %s", want, msg)
		}
	}
}

func TestLoadPlanRejectsUnknownFieldsOnSecondPass(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(planPath, []byte(`{
  "schema_version": 1,
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
		SchemaVersion: 1,
		Name:          "timeout",
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
		SchemaVersion: 1,
		Name:          "bad",
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
		SchemaVersion: 1,
		Name:          "cycle",
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
		SchemaVersion: 1,
		Name:          "state",
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

func TestNewPlanFileGeneratesValidSchemaV1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := NewPlanFile("my-plan", dir)
	if err != nil {
		t.Fatalf("new plan file: %v", err)
	}
	plan, err := LoadPlan(path)
	if err != nil {
		t.Fatalf("load generated plan: %v", err)
	}
	if plan.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1, got %d", plan.SchemaVersion)
	}
}
