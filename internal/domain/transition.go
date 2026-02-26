package domain

import "fmt"

// ValidTransitions defines the allowed state changes for a task.
// Terminal states (done, failed) have no outgoing transitions.
var ValidTransitions = map[TaskStatus][]TaskStatus{
	TaskPending:   {TaskExecuting, TaskFailed},
	TaskExecuting: {TaskReviewing, TaskDone, TaskPending, TaskFailed},
	TaskReviewing: {TaskDone, TaskPending, TaskFailed},
	TaskDone:      {},
	TaskFailed:    {},
}

// AllStatuses enumerates every valid TaskStatus for completeness checks.
var AllStatuses = []TaskStatus{
	TaskPending,
	TaskExecuting,
	TaskReviewing,
	TaskDone,
	TaskFailed,
}

// Transition validates a state change from → to.
// Returns nil if valid, or an error describing the invalid transition.
func Transition(from, to TaskStatus) error {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return fmt.Errorf("unknown source state %q", from)
	}
	for _, a := range allowed {
		if a == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition %s → %s", from, to)
}

// IsTerminal reports whether a status is a terminal (final) state.
func IsTerminal(status TaskStatus) bool {
	return status == TaskDone || status == TaskFailed
}

// NormalizeStatus maps legacy status values to current ones and
// resets transient states for crash recovery.
func NormalizeStatus(status TaskStatus) TaskStatus {
	switch status {
	case TaskStatusOpen:
		return TaskPending
	case TaskExecuting, TaskReviewing:
		// Transient states on load mean the process crashed mid-flight.
		return TaskPending
	case TaskPending, TaskDone, TaskFailed:
		return status
	default:
		return TaskPending
	}
}
