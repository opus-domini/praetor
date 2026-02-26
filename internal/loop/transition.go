package loop

import "github.com/opus-domini/praetor/internal/domain"

// validTransitions — re-exported for internal loop use.
var validTransitions = domain.ValidTransitions

// allStatuses — re-exported for internal loop use.
var allStatuses = domain.AllStatuses

// Transition delegates to domain.Transition.
func Transition(from, to TaskStatus) error {
	return domain.Transition(from, to)
}

// IsTerminal delegates to domain.IsTerminal.
func IsTerminal(status TaskStatus) bool {
	return domain.IsTerminal(status)
}

// NormalizeStatus delegates to domain.NormalizeStatus.
func NormalizeStatus(status TaskStatus) TaskStatus {
	return domain.NormalizeStatus(status)
}
