package pipeline

import "testing"

func TestCanFollow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		current Phase
		next    Phase
		ok      bool
	}{
		{PhasePlan, PhaseExecute, true},
		{PhaseExecute, PhaseReview, true},
		{PhaseExecute, PhaseGate, true},
		{PhaseReview, PhaseGate, true},
		{PhaseGate, PhaseExecute, true},
		{PhaseGate, PhasePlan, true},
		// Invalid transitions.
		{PhasePlan, PhaseReview, false},
		{PhasePlan, PhaseGate, false},
		{PhaseReview, PhaseExecute, false},
		{PhaseReview, PhasePlan, false},
	}

	for _, tt := range tests {
		got := CanFollow(tt.current, tt.next)
		if got != tt.ok {
			t.Errorf("CanFollow(%q, %q) = %v, want %v", tt.current, tt.next, got, tt.ok)
		}
	}
}

func TestSuccessorsCompleteness(t *testing.T) {
	t.Parallel()

	for _, phase := range AllPhases {
		succs := Successors(phase)
		if succs == nil {
			t.Errorf("Successors(%q) returned nil", phase)
		}
	}
}
