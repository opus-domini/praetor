package fsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/opus-domini/praetor/internal/orchestration/fsm"
)

// counterState tracks how many transitions have occurred and records their order.
type counterState struct {
	count  int
	visits []string
}

func TestRunCompletesOnNil(t *testing.T) {
	state := &counterState{}

	done := func(_ context.Context, s *counterState) (fsm.StateFn[counterState], error) {
		s.count++
		return nil, nil // signal completion
	}

	if err := fsm.Run(context.Background(), state, done); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state.count != 1 {
		t.Fatalf("expected count 1, got %d", state.count)
	}
}

func TestRunPropagatesErrors(t *testing.T) {
	state := &counterState{}
	sentinel := errors.New("boom")

	failing := func(_ context.Context, _ *counterState) (fsm.StateFn[counterState], error) {
		return nil, sentinel
	}

	err := fsm.Run(context.Background(), state, failing)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestRunRespectsContextCancellation(t *testing.T) {
	state := &counterState{}
	ctx, cancel := context.WithCancel(context.Background())

	var looping fsm.StateFn[counterState]
	looping = func(ctx context.Context, s *counterState) (fsm.StateFn[counterState], error) {
		s.count++
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Cancel after the first transition so the next invocation sees it.
		if s.count == 1 {
			cancel()
		}
		return looping, nil
	}

	err := fsm.Run(ctx, state, looping)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if state.count != 2 {
		t.Fatalf("expected count 2 (one normal + one that observed cancellation), got %d", state.count)
	}
}

func TestRunTransitionsInOrder(t *testing.T) {
	state := &counterState{}

	var stepA, stepB, stepC fsm.StateFn[counterState]

	stepA = func(_ context.Context, s *counterState) (fsm.StateFn[counterState], error) {
		s.visits = append(s.visits, "A")
		return stepB, nil
	}
	stepB = func(_ context.Context, s *counterState) (fsm.StateFn[counterState], error) {
		s.visits = append(s.visits, "B")
		return stepC, nil
	}
	stepC = func(_ context.Context, s *counterState) (fsm.StateFn[counterState], error) {
		s.visits = append(s.visits, "C")
		return nil, nil
	}

	if err := fsm.Run(context.Background(), state, stepA); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := []string{"A", "B", "C"}
	if len(state.visits) != len(expected) {
		t.Fatalf("expected %d visits, got %d", len(expected), len(state.visits))
	}
	for i, v := range expected {
		if state.visits[i] != v {
			t.Fatalf("visit[%d]: expected %q, got %q", i, v, state.visits[i])
		}
	}
}
