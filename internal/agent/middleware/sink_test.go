package middleware

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNopSink(t *testing.T) {
	t.Parallel()
	var s NopSink
	s.Emit(ExecutionEvent{Type: EventAgentComplete})
	// No panic, no side effects.
}

func TestJSONLSinkWritesEvents(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, err := NewJSONLSink(path)
	if err != nil {
		t.Fatalf("create sink: %v", err)
	}

	sink.Emit(ExecutionEvent{Type: EventAgentStart, Agent: "claude"})
	sink.Emit(ExecutionEvent{Type: EventAgentComplete, Agent: "claude", DurationS: 1.5})

	if err := sink.Close(); err != nil {
		t.Fatalf("close sink: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %s", len(lines), string(data))
	}

	var ev ExecutionEvent
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if ev.Type != EventAgentStart {
		t.Fatalf("expected agent_start, got %q", ev.Type)
	}
	if ev.Agent != "claude" {
		t.Fatalf("expected agent=claude, got %q", ev.Agent)
	}

	if err := json.Unmarshal([]byte(lines[1]), &ev); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if ev.DurationS != 1.5 {
		t.Fatalf("expected duration_s=1.5, got %f", ev.DurationS)
	}
}

func TestJSONLSinkInvalidPath(t *testing.T) {
	t.Parallel()
	_, err := NewJSONLSink("/nonexistent/dir/events.jsonl")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestMultiplexSink(t *testing.T) {
	t.Parallel()
	c1 := &CollectorSink{}
	c2 := &CollectorSink{}
	mux := NewMultiplexSink(c1, c2)

	mux.Emit(ExecutionEvent{Type: EventAgentComplete, Agent: "ollama"})

	if c1.Len() != 1 {
		t.Fatalf("expected 1 event in c1, got %d", c1.Len())
	}
	if c2.Len() != 1 {
		t.Fatalf("expected 1 event in c2, got %d", c2.Len())
	}
	if c1.Events[0].Agent != "ollama" {
		t.Fatalf("expected agent=ollama in c1")
	}
}

func TestMultiplexSinkEmpty(t *testing.T) {
	t.Parallel()
	mux := NewMultiplexSink()
	mux.Emit(ExecutionEvent{Type: EventAgentStart})
	// No panic with zero sinks.
}

func TestCollectorSink(t *testing.T) {
	t.Parallel()
	c := &CollectorSink{}
	if c.Len() != 0 {
		t.Fatal("expected empty collector")
	}
	c.Emit(ExecutionEvent{Type: EventAgentStart})
	c.Emit(ExecutionEvent{Type: EventAgentComplete})
	if c.Len() != 2 {
		t.Fatalf("expected 2 events, got %d", c.Len())
	}
}
