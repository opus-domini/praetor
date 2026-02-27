package middleware

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// EventSink receives execution events for observability.
type EventSink interface {
	Emit(event ExecutionEvent)
}

// NopSink discards all events. Useful for tests and disabled observability.
type NopSink struct{}

func (NopSink) Emit(ExecutionEvent) {}

// JSONLSink appends events as JSON lines to a file.
type JSONLSink struct {
	mu   sync.Mutex
	file *os.File
}

// NewJSONLSink creates a sink that appends to the given file path.
// The file is created if it doesn't exist.
func NewJSONLSink(path string) (*JSONLSink, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open event sink %s: %w", path, err)
	}
	return &JSONLSink{file: f}, nil
}

// Emit writes one event as a JSON line.
func (s *JSONLSink) Emit(event ExecutionEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.file.Write(data)
}

// Close flushes and closes the underlying file.
func (s *JSONLSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file.Close()
}

// MultiplexSink fans out events to multiple sinks.
type MultiplexSink struct {
	sinks []EventSink
}

// NewMultiplexSink creates a sink that broadcasts to all children.
func NewMultiplexSink(sinks ...EventSink) *MultiplexSink {
	return &MultiplexSink{sinks: sinks}
}

// Emit sends the event to every child sink.
func (m *MultiplexSink) Emit(event ExecutionEvent) {
	for _, s := range m.sinks {
		s.Emit(event)
	}
}

// CollectorSink accumulates events in memory. Useful for tests.
type CollectorSink struct {
	mu     sync.Mutex
	Events []ExecutionEvent
}

// Emit appends the event to the in-memory slice.
func (c *CollectorSink) Emit(event ExecutionEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Events = append(c.Events, event)
}

// Len returns the number of captured events.
func (c *CollectorSink) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Events)
}
