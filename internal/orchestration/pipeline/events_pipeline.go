package pipeline

import (
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
)

func emitTaskEvent(run *activeRun, eventType middleware.EventType, taskID, phase, message string, actor *domain.EventActor, data map[string]any) {
	if run == nil || run.eventSink == nil {
		return
	}
	event := middleware.ExecutionEvent{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Type:          eventType,
		EventType:     string(eventType),
		RunID:         run.runID,
		TaskID:        strings.TrimSpace(taskID),
		Phase:         strings.TrimSpace(phase),
		Message:       strings.TrimSpace(message),
		Actor:         actor,
		Data:          data,
	}
	if actor != nil {
		event.Agent = strings.TrimSpace(actor.Agent)
		event.Role = strings.TrimSpace(actor.Role)
	}
	run.eventSink.Emit(event)
}
