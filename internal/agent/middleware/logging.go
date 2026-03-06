package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
)

// Logger receives structured log entries from the logging middleware.
type Logger interface {
	Log(entry LogEntry)
}

// LogEntry describes one agent invocation for logging purposes.
type LogEntry struct {
	Timestamp string
	Agent     string
	Role      string
	Status    string
	Error     string
	Strategy  string
	DurationS float64
	CostUSD   float64
}

// Logging creates a middleware that logs every agent invocation and optionally
// emits events to an EventSink (may be nil to skip event emission).
func Logging(logger Logger, sink EventSink) Middleware {
	return func(next domain.AgentRuntime) domain.AgentRuntime {
		return runtimeFunc(func(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
			start := time.Now()
			actor := &domain.EventActor{
				Role:  req.Role,
				Agent: string(req.Agent),
				Model: strings.TrimSpace(req.Model),
			}
			if sink != nil {
				sink.Emit(ExecutionEvent{
					SchemaVersion: 1,
					Timestamp:     start.UTC().Format(time.RFC3339),
					Type:          EventAgentStart,
					EventType:     string(EventAgentStart),
					Agent:         string(req.Agent),
					TaskID:        req.TaskLabel,
					Phase:         req.Role,
					Role:          req.Role,
					Actor:         actor,
				})
			}
			result, err := next.Run(ctx, req)
			duration := time.Since(start).Seconds()

			status := "ok"
			errMsg := ""
			eventType := EventAgentComplete
			if err != nil {
				status = "error"
				errMsg = err.Error()
				eventType = EventAgentError
			}

			entry := LogEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Agent:     string(req.Agent),
				Role:      req.Role,
				Status:    status,
				Error:     errMsg,
				Strategy:  string(result.Strategy),
				DurationS: duration,
				CostUSD:   result.CostUSD,
			}
			logger.Log(entry)

			if sink != nil {
				sink.Emit(ExecutionEvent{
					SchemaVersion: 1,
					Timestamp:     entry.Timestamp,
					Type:          eventType,
					EventType:     string(eventType),
					Agent:         entry.Agent,
					TaskID:        req.TaskLabel,
					Phase:         req.Role,
					Role:          entry.Role,
					Actor:         actor,
					Strategy:      entry.Strategy,
					Error:         entry.Error,
					DurationS:     entry.DurationS,
					CostUSD:       entry.CostUSD,
				})
			}

			return result, err
		})
	}
}
