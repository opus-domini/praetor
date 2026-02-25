package loop

import (
	"strings"
	"testing"
)

func TestValidatePlanSuccess(t *testing.T) {
	t.Parallel()

	plan := Plan{
		Tasks: []Task{
			{ID: "TASK-001", Title: "First task", Executor: AgentCodex, Reviewer: AgentClaude},
			{ID: "TASK-002", Title: "Second task", DependsOn: []string{"TASK-001"}},
		},
	}

	if err := ValidatePlan(plan); err != nil {
		t.Fatalf("validate plan: %v", err)
	}
}

func TestValidatePlanUnknownDependency(t *testing.T) {
	t.Parallel()

	plan := Plan{Tasks: []Task{{ID: "TASK-001", Title: "First task", DependsOn: []string{"TASK-999"}}}}
	err := ValidatePlan(plan)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "unknown task id") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidatePlanDuplicateID(t *testing.T) {
	t.Parallel()

	plan := Plan{
		Tasks: []Task{
			{ID: "TASK-001", Title: "One"},
			{ID: "TASK-001", Title: "Two"},
		},
	}

	err := ValidatePlan(plan)
	if err == nil {
		t.Fatalf("expected duplicate id error")
	}
	if !strings.Contains(err.Error(), "duplicated id") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidatePlanAcceptsProviderSpecificModel(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Tasks: []Task{
			{Title: "test task", Model: "gpt-4"},
		},
	}
	if err := ValidatePlan(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePlanValidModel(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Tasks: []Task{
			{Title: "test task", Model: "opus"},
		},
	}
	if err := ValidatePlan(plan); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
