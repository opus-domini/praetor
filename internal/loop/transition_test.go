package loop

import "testing"

func TestTransitionValidMoves(t *testing.T) {
	t.Parallel()

	valid := []struct {
		from, to TaskStatus
	}{
		{TaskPending, TaskExecuting},
		{TaskPending, TaskFailed},
		{TaskExecuting, TaskReviewing},
		{TaskExecuting, TaskDone},
		{TaskExecuting, TaskPending},
		{TaskExecuting, TaskFailed},
		{TaskReviewing, TaskDone},
		{TaskReviewing, TaskPending},
		{TaskReviewing, TaskFailed},
	}

	for _, tt := range valid {
		t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
			t.Parallel()
			if err := Transition(tt.from, tt.to); err != nil {
				t.Fatalf("expected valid transition %s → %s, got error: %v", tt.from, tt.to, err)
			}
		})
	}
}

func TestTransitionInvalidMoves(t *testing.T) {
	t.Parallel()

	invalid := []struct {
		from, to TaskStatus
	}{
		// Terminal states have no outgoing transitions.
		{TaskDone, TaskPending},
		{TaskDone, TaskExecuting},
		{TaskFailed, TaskPending},
		{TaskFailed, TaskDone},
		// Skip-state transitions are invalid.
		{TaskPending, TaskReviewing},
		{TaskPending, TaskDone},
		// Self-transitions are not in the table.
		{TaskPending, TaskPending},
		{TaskExecuting, TaskExecuting},
	}

	for _, tt := range invalid {
		t.Run(string(tt.from)+"→"+string(tt.to), func(t *testing.T) {
			t.Parallel()
			if err := Transition(tt.from, tt.to); err == nil {
				t.Fatalf("expected invalid transition %s → %s to be rejected", tt.from, tt.to)
			}
		})
	}
}

func TestTransitionTableCompleteness(t *testing.T) {
	t.Parallel()

	for _, status := range allStatuses {
		if _, ok := validTransitions[status]; !ok {
			t.Fatalf("status %q missing from transition table", status)
		}
	}
}

func TestTransitionUnknownSource(t *testing.T) {
	t.Parallel()

	if err := Transition("bogus", TaskDone); err == nil {
		t.Fatal("expected error for unknown source state")
	}
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   TaskStatus
		terminal bool
	}{
		{TaskPending, false},
		{TaskExecuting, false},
		{TaskReviewing, false},
		{TaskDone, true},
		{TaskFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			if got := IsTerminal(tt.status); got != tt.terminal {
				t.Fatalf("IsTerminal(%s) = %v, want %v", tt.status, got, tt.terminal)
			}
		})
	}
}

func TestNormalizeStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input, want TaskStatus
	}{
		{TaskStatusOpen, TaskPending},
		{TaskPending, TaskPending},
		{TaskExecuting, TaskPending}, // crash recovery
		{TaskReviewing, TaskPending}, // crash recovery
		{TaskDone, TaskDone},
		{TaskFailed, TaskFailed},
		{"unknown", TaskPending},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			t.Parallel()
			if got := NormalizeStatus(tt.input); got != tt.want {
				t.Fatalf("NormalizeStatus(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestStateCounters(t *testing.T) {
	t.Parallel()

	state := State{
		Tasks: []StateTask{
			{ID: "A", Status: TaskDone},
			{ID: "B", Status: TaskPending},
			{ID: "C", Status: TaskExecuting},
			{ID: "D", Status: TaskFailed},
			{ID: "E", Status: TaskReviewing},
		},
	}

	if got := state.DoneCount(); got != 1 {
		t.Fatalf("DoneCount = %d, want 1", got)
	}
	if got := state.FailedCount(); got != 1 {
		t.Fatalf("FailedCount = %d, want 1", got)
	}
	if got := state.ActiveCount(); got != 3 {
		t.Fatalf("ActiveCount = %d, want 3", got)
	}
	if got := state.OpenCount(); got != 3 {
		t.Fatalf("OpenCount = %d, want 3 (same as ActiveCount)", got)
	}
}
