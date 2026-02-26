package agents

import (
	"context"
	"testing"
)

type fakeAgent struct{ id ID }

func (f fakeAgent) ID() ID                     { return f.id }
func (f fakeAgent) Capabilities() Capabilities { return Capabilities{} }
func (f fakeAgent) Plan(context.Context, PlanRequest) (PlanResponse, error) {
	return PlanResponse{}, nil
}
func (f fakeAgent) Execute(context.Context, ExecuteRequest) (ExecuteResponse, error) {
	return ExecuteResponse{}, nil
}
func (f fakeAgent) Review(context.Context, ReviewRequest) (ReviewResponse, error) {
	return ReviewResponse{}, nil
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	if err := r.Register(fakeAgent{id: Codex}); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := r.Register(fakeAgent{id: Codex}); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}
