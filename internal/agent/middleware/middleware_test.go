package middleware

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
)

// stubRuntime returns a fixed result or error.
type stubRuntime struct {
	result domain.AgentResult
	err    error
}

func (s *stubRuntime) Run(_ context.Context, _ domain.AgentRequest) (domain.AgentResult, error) {
	return s.result, s.err
}

func TestChainEmpty(t *testing.T) {
	t.Parallel()
	base := &stubRuntime{result: domain.AgentResult{Output: "hello"}}
	rt := Chain(base)
	res, err := rt.Run(context.Background(), domain.AgentRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "hello" {
		t.Fatalf("expected hello, got %q", res.Output)
	}
}

func TestChainOrder(t *testing.T) {
	t.Parallel()
	var order []string
	var mu sync.Mutex

	mwA := func(next domain.AgentRuntime) domain.AgentRuntime {
		return runtimeFunc(func(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
			mu.Lock()
			order = append(order, "A-before")
			mu.Unlock()
			res, err := next.Run(ctx, req)
			mu.Lock()
			order = append(order, "A-after")
			mu.Unlock()
			return res, err
		})
	}
	mwB := func(next domain.AgentRuntime) domain.AgentRuntime {
		return runtimeFunc(func(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
			mu.Lock()
			order = append(order, "B-before")
			mu.Unlock()
			res, err := next.Run(ctx, req)
			mu.Lock()
			order = append(order, "B-after")
			mu.Unlock()
			return res, err
		})
	}

	base := &stubRuntime{result: domain.AgentResult{Output: "ok"}}
	rt := Chain(base, mwA, mwB)
	_, err := rt.Run(context.Background(), domain.AgentRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"A-before", "B-before", "B-after", "A-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, want := range expected {
		if order[i] != want {
			t.Fatalf("order[%d] = %q, want %q", i, order[i], want)
		}
	}
}

type testLogger struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (l *testLogger) Log(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func TestLoggingMiddlewareSuccess(t *testing.T) {
	t.Parallel()
	logger := &testLogger{}
	collector := &CollectorSink{}
	base := &stubRuntime{result: domain.AgentResult{CostUSD: 0.01, Strategy: domain.ExecutionStrategyProcess}}

	rt := Logging(logger, collector)(base)
	req := domain.AgentRequest{Agent: "claude", Role: "execute"}
	_, err := rt.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(logger.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logger.entries))
	}
	entry := logger.entries[0]
	if entry.Agent != "claude" {
		t.Fatalf("expected agent=claude, got %q", entry.Agent)
	}
	if entry.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", entry.Status)
	}
	if entry.CostUSD != 0.01 {
		t.Fatalf("expected cost=0.01, got %f", entry.CostUSD)
	}
	if entry.Error != "" {
		t.Fatalf("expected no error, got %q", entry.Error)
	}

	if collector.Len() != 1 {
		t.Fatalf("expected 1 event, got %d", collector.Len())
	}
	ev := collector.Events[0]
	if ev.Type != EventAgentComplete {
		t.Fatalf("expected event type agent_complete, got %q", ev.Type)
	}
}

func TestLoggingMiddlewareError(t *testing.T) {
	t.Parallel()
	logger := &testLogger{}
	collector := &CollectorSink{}
	base := &stubRuntime{err: fmt.Errorf("connection refused")}

	rt := Logging(logger, collector)(base)
	_, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "ollama", Role: "execute"})
	if err == nil {
		t.Fatal("expected error")
	}

	if logger.entries[0].Status != "error" {
		t.Fatalf("expected status=error, got %q", logger.entries[0].Status)
	}
	if logger.entries[0].Error == "" {
		t.Fatal("expected error message")
	}

	if collector.Events[0].Type != EventAgentError {
		t.Fatalf("expected event type agent_error, got %q", collector.Events[0].Type)
	}
}

func TestLoggingMiddlewareNilSink(t *testing.T) {
	t.Parallel()
	logger := &testLogger{}
	base := &stubRuntime{result: domain.AgentResult{}}

	rt := Logging(logger, nil)(base)
	_, err := rt.Run(context.Background(), domain.AgentRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logger.entries) != 1 {
		t.Fatalf("expected 1 log entry with nil sink")
	}
}

func TestMetricsMiddleware(t *testing.T) {
	t.Parallel()
	counters := NewCounters()
	base := &stubRuntime{result: domain.AgentResult{CostUSD: 0.05}}

	rt := Metrics(counters)(base)
	for range 3 {
		_, _ = rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})
	}
	_, _ = rt.Run(context.Background(), domain.AgentRequest{Agent: "ollama", Role: "review"})

	counts, totalCost, totalCalls := counters.Snapshot()
	if totalCalls != 4 {
		t.Fatalf("expected 4 total calls, got %d", totalCalls)
	}
	if totalCost != 0.20 {
		t.Fatalf("expected total cost 0.20, got %f", totalCost)
	}
	if counts[CounterKey{Agent: "claude", Role: "execute", Status: "ok"}] != 3 {
		t.Fatal("expected 3 claude/execute/ok calls")
	}
	if counts[CounterKey{Agent: "ollama", Role: "review", Status: "ok"}] != 1 {
		t.Fatal("expected 1 ollama/review/ok call")
	}
}

func TestMetricsMiddlewareError(t *testing.T) {
	t.Parallel()
	counters := NewCounters()
	base := &stubRuntime{err: fmt.Errorf("fail"), result: domain.AgentResult{CostUSD: 0.01}}

	rt := Metrics(counters)(base)
	_, _ = rt.Run(context.Background(), domain.AgentRequest{Agent: "claude", Role: "execute"})

	counts, _, totalCalls := counters.Snapshot()
	if totalCalls != 1 {
		t.Fatalf("expected 1 call, got %d", totalCalls)
	}
	if counts[CounterKey{Agent: "claude", Role: "execute", Status: "error"}] != 1 {
		t.Fatal("expected 1 error count")
	}
}

func TestFullChain(t *testing.T) {
	t.Parallel()
	logger := &testLogger{}
	counters := NewCounters()
	collector := &CollectorSink{}
	base := &stubRuntime{result: domain.AgentResult{Output: "done", CostUSD: 0.02}}

	rt := Chain(base, Logging(logger, collector), Metrics(counters))
	res, err := rt.Run(context.Background(), domain.AgentRequest{Agent: "gemini", Role: "execute"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output != "done" {
		t.Fatalf("expected output=done, got %q", res.Output)
	}

	if len(logger.entries) != 1 {
		t.Fatal("expected 1 log entry")
	}
	_, _, totalCalls := counters.Snapshot()
	if totalCalls != 1 {
		t.Fatal("expected 1 metric count")
	}
	if collector.Len() != 1 {
		t.Fatal("expected 1 event")
	}
}
