package domain

import (
	"reflect"
	"testing"
)

func TestRunnableTaskIndicesRespectsWaveOrderAndLimit(t *testing.T) {
	t.Parallel()

	state := State{
		Tasks: []StateTask{
			{ID: "TASK-001", Status: TaskPending},
			{ID: "TASK-002", Status: TaskPending},
			{ID: "TASK-003", Status: TaskPending, DependsOn: []string{"TASK-001"}},
			{ID: "TASK-004", Status: TaskPending, DependsOn: []string{"TASK-001", "TASK-002"}},
		},
	}

	if got := RunnableTaskIndices(state, 0); !reflect.DeepEqual(got, []int{0, 1}) {
		t.Fatalf("runnable indices = %v, want [0 1]", got)
	}
	if got := RunnableTaskIndices(state, 1); !reflect.DeepEqual(got, []int{0}) {
		t.Fatalf("limited runnable indices = %v, want [0]", got)
	}

	state.Tasks[0].Status = TaskDone
	state.Tasks[1].Status = TaskDone
	if got := RunnableTaskIndices(state, 0); !reflect.DeepEqual(got, []int{2, 3}) {
		t.Fatalf("next wave indices = %v, want [2 3]", got)
	}
}
