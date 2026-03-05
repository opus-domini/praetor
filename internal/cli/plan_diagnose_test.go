package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilterEventsByQuery(t *testing.T) {
	t.Parallel()
	events := []map[string]any{
		{"event_type": "agent_error", "timestamp": "2026-01-01T10:00:01Z"},
		{"event_type": "task_stalled", "timestamp": "2026-01-01T10:00:02Z"},
		{"event_type": "agent_fallback", "timestamp": "2026-01-01T10:00:03Z"},
	}
	if got := len(filterEventsByQuery(events, "errors")); got != 1 {
		t.Fatalf("errors query expected 1, got %d", got)
	}
	if got := len(filterEventsByQuery(events, "stalls")); got != 1 {
		t.Fatalf("stalls query expected 1, got %d", got)
	}
	if got := len(filterEventsByQuery(events, "fallbacks")); got != 1 {
		t.Fatalf("fallbacks query expected 1, got %d", got)
	}
	if got := len(filterEventsByQuery(events, "all")); got != 3 {
		t.Fatalf("all query expected 3, got %d", got)
	}
}

func TestBuildDiagnoseSummary(t *testing.T) {
	t.Parallel()
	events := []map[string]any{
		{"event_type": "agent_error", "cost_usd": 0.5},
		{"event_type": "task_stalled"},
		{"event_type": "agent_fallback"},
		{"event_type": "gate_result", "action": "FAIL"},
	}
	perf := []map[string]any{
		{"prompt_chars": 1000, "estimated_tokens": 250},
		{"prompt_chars": 2000, "estimated_tokens": 500},
	}

	summary := buildDiagnoseSummary(events, perf)
	if summary.EventsTotal != 4 {
		t.Fatalf("events_total = %d, want 4", summary.EventsTotal)
	}
	if summary.Errors != 1 {
		t.Fatalf("errors = %d, want 1", summary.Errors)
	}
	if summary.Stalls != 1 {
		t.Fatalf("stalls = %d, want 1", summary.Stalls)
	}
	if summary.Fallbacks != 1 {
		t.Fatalf("fallbacks = %d, want 1", summary.Fallbacks)
	}
	if summary.GateFailures != 1 {
		t.Fatalf("gate_failures = %d, want 1", summary.GateFailures)
	}
	if summary.AvgPromptChars != 1500 {
		t.Fatalf("avg_prompt_chars = %.2f, want 1500", summary.AvgPromptChars)
	}
}

func TestBuildDiagnoseRegression(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	if err := os.WriteFile(baselinePath, []byte(`{
  "errors": 0,
  "stalls": 0,
  "gate_failures": 0,
  "total_cost_usd": 1.0,
  "avg_prompt_chars": 1000,
  "avg_estimated_tokens": 250
}`), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	current := diagnoseSummary{
		Errors:             1,
		Stalls:             0,
		GateFailures:       0,
		TotalCostUSD:       2.0,
		AvgPromptChars:     1100,
		AvgEstimatedTokens: 260,
	}
	regression, err := buildDiagnoseRegression(baselinePath, current)
	if err != nil {
		t.Fatalf("build regression: %v", err)
	}
	if regression.Verdict != "fail" {
		t.Fatalf("verdict = %q, want fail", regression.Verdict)
	}
	if len(regression.Checks) == 0 {
		t.Fatal("expected regression checks")
	}
}
