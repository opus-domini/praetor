package loop

import "testing"

func TestNextRunnableTaskRespectsDependencies(t *testing.T) {
	t.Parallel()

	state := State{
		Tasks: []StateTask{
			{ID: "TASK-001", Title: "A", Status: TaskDone},
			{ID: "TASK-002", Title: "B", DependsOn: []string{"TASK-001"}, Status: TaskPending},
			{ID: "TASK-003", Title: "C", DependsOn: []string{"TASK-999"}, Status: TaskPending},
		},
	}

	index, task, ok := NextRunnableTask(state)
	if !ok {
		t.Fatalf("expected runnable task")
	}
	if index != 1 || task.ID != "TASK-002" {
		t.Fatalf("unexpected runnable task: index=%d id=%s", index, task.ID)
	}
}

func TestBlockedTasksReport(t *testing.T) {
	t.Parallel()

	state := State{
		Tasks: []StateTask{
			{ID: "TASK-001", Title: "A", Status: TaskPending, DependsOn: []string{"TASK-002"}},
			{ID: "TASK-002", Title: "B", Status: TaskPending, DependsOn: []string{"TASK-003"}},
		},
	}

	report := BlockedTasksReport(state, 5)
	if len(report) != 2 {
		t.Fatalf("unexpected report size: %d", len(report))
	}
}
