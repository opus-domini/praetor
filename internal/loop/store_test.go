package loop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrInitializeStateMergesPlanChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan.json")

	plan := Plan{
		Title: "sample",
		Tasks: []Task{
			{ID: "TASK-001", Title: "First task"},
			{ID: "TASK-002", Title: "Second task", DependsOn: []string{"TASK-001"}},
		},
	}
	writePlanFile(t, planPath, plan)

	store := NewStore(filepath.Join(tmpDir, "state"))
	state, err := store.LoadOrInitializeState(planPath, plan)
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	state.Tasks[0].Status = TaskStatusDone
	if err := store.WriteState(planPath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	plan.Tasks = append(plan.Tasks, Task{ID: "TASK-003", Title: "Third task", DependsOn: []string{"TASK-002"}})
	writePlanFile(t, planPath, plan)

	merged, err := store.LoadOrInitializeState(planPath, plan)
	if err != nil {
		t.Fatalf("merge state: %v", err)
	}

	if len(merged.Tasks) != 3 {
		t.Fatalf("unexpected merged task count: %d", len(merged.Tasks))
	}
	if merged.Tasks[0].Status != TaskStatusDone {
		t.Fatalf("expected first task to remain done")
	}
	if merged.Tasks[2].Status != TaskStatusOpen {
		t.Fatalf("expected new task to be open")
	}
}

func writePlanFile(t *testing.T, path string, plan Plan) {
	t.Helper()
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("encode plan: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}
