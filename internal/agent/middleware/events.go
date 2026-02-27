package middleware

// EventType identifies the kind of execution event.
type EventType string

const (
	EventAgentStart    EventType = "agent_start"
	EventAgentComplete EventType = "agent_complete"
	EventAgentError    EventType = "agent_error"
	EventAgentFallback EventType = "agent_fallback"
)

// ExecutionEvent captures one observable moment during agent execution.
type ExecutionEvent struct {
	Timestamp string    `json:"timestamp"`
	Type      EventType `json:"type"`
	Agent     string    `json:"agent"`
	Role      string    `json:"role,omitempty"`
	Strategy  string    `json:"strategy,omitempty"`
	Error     string    `json:"error,omitempty"`
	Message   string    `json:"message,omitempty"`
	DurationS float64   `json:"duration_s,omitempty"`
	CostUSD   float64   `json:"cost_usd,omitempty"`
}
