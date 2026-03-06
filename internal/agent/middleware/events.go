package middleware

import "github.com/opus-domini/praetor/internal/domain"

// EventType identifies the kind of execution event.
type EventType string

const (
	EventAgentStart          EventType = "agent_start"
	EventAgentComplete       EventType = "agent_complete"
	EventAgentError          EventType = "agent_error"
	EventAgentFallback       EventType = "agent_fallback"
	EventTaskStarted         EventType = "task_started"
	EventTaskCompleted       EventType = "task_completed"
	EventTaskFailed          EventType = "task_failed"
	EventTaskStalled         EventType = "task_stalled"
	EventPromptBudgetWarning EventType = "prompt_budget_warning"
	EventCostBudgetWarning   EventType = "cost_budget_warning"
	EventCostBudgetExceeded  EventType = "cost_budget_exceeded"
	EventGateResult          EventType = "gate_result"
	EventParallelMerge       EventType = "parallel_merge"
	EventParallelConflict    EventType = "parallel_conflict"
	EventStateTransit        EventType = "state_transition"
)

// ExecutionEvent captures one observable moment during agent execution.
type ExecutionEvent struct {
	SchemaVersion int                `json:"schema_version,omitempty"`
	Timestamp     string             `json:"timestamp"`
	Type          EventType          `json:"type"`
	EventType     string             `json:"event_type,omitempty"`
	RunID         string             `json:"run_id,omitempty"`
	TaskID        string             `json:"task_id,omitempty"`
	Phase         string             `json:"phase,omitempty"`
	Agent         string             `json:"agent,omitempty"`
	Role          string             `json:"role,omitempty"`
	Actor         *domain.EventActor `json:"actor,omitempty"`
	Strategy      string             `json:"strategy,omitempty"`
	Error         string             `json:"error,omitempty"`
	Message       string             `json:"message,omitempty"`
	DurationS     float64            `json:"duration_s,omitempty"`
	CostUSD       float64            `json:"cost_usd,omitempty"`
	Similarity    float64            `json:"similarity,omitempty"`
	WindowSize    int                `json:"window_size,omitempty"`
	Action        string             `json:"action,omitempty"`
	Data          map[string]any     `json:"data,omitempty"`
}
