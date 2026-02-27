package cli

import "testing"

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
