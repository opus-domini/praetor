package orchestrator

import (
	"context"
	"errors"
	"testing"
)

type testProvider struct {
	id ProviderID
}

func (p testProvider) ID() ProviderID {
	return p.id
}

func (p testProvider) Run(_ context.Context, req Request) (Result, error) {
	if req.Prompt == "boom" {
		return Result{}, errors.New("provider failure")
	}
	return Result{
		Provider: p.id,
		Response: "ok:" + req.Prompt,
	}, nil
}

func TestEngineRun(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	if err := registry.Register(testProvider{id: ProviderCodex}); err != nil {
		t.Fatalf("register provider: %v", err)
	}

	engine := New(registry)
	result, err := engine.Run(context.Background(), Request{
		Provider: " codex ",
		Prompt:   "  hello  ",
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if result.Provider != ProviderCodex {
		t.Fatalf("unexpected provider: %s", result.Provider)
	}
	if result.Response != "ok:hello" {
		t.Fatalf("unexpected response: %q", result.Response)
	}
}

func TestEngineRunUnknownProvider(t *testing.T) {
	t.Parallel()

	engine := New(NewRegistry())
	_, err := engine.Run(context.Background(), Request{
		Provider: ProviderCodex,
		Prompt:   "hello",
	})
	if err == nil {
		t.Fatalf("expected unknown provider error")
	}
}

func TestRegistryRejectsDuplicateProvider(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	if err := registry.Register(testProvider{id: ProviderClaude}); err != nil {
		t.Fatalf("register first provider: %v", err)
	}
	if err := registry.Register(testProvider{id: ProviderClaude}); err == nil {
		t.Fatalf("expected duplicate provider error")
	}
}

func TestEngineRunZeroValueReturnsError(t *testing.T) {
	t.Parallel()

	var engine Engine
	_, err := engine.Run(context.Background(), Request{
		Provider: ProviderCodex,
		Prompt:   "hello",
	})
	if err == nil {
		t.Fatal("expected error for zero-value engine")
	}
}
