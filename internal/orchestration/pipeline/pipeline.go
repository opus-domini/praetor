// Package pipeline defines the Plan/Execute/Review phase sequencing rules
// for the cognitive orchestration loop described in RFC-009 §5.4.
package pipeline

// Phase identifies one step in the Plan-Execute-Review pipeline.
type Phase string

const (
	PhasePlan    Phase = "plan"
	PhaseExecute Phase = "execute"
	PhaseReview  Phase = "review"
	PhaseGate    Phase = "gate"
)

// validSuccessors defines which phases may follow each phase.
var validSuccessors = map[Phase][]Phase{
	PhasePlan:    {PhaseExecute},
	PhaseExecute: {PhaseReview, PhaseGate},
	PhaseReview:  {PhaseGate},
	PhaseGate:    {PhaseExecute, PhasePlan}, // retry or re-plan
}

// CanFollow reports whether next is a valid successor of current.
func CanFollow(current, next Phase) bool {
	for _, s := range validSuccessors[current] {
		if s == next {
			return true
		}
	}
	return false
}

// Successors returns the valid successors for a given phase.
func Successors(phase Phase) []Phase {
	return validSuccessors[phase]
}

// AllPhases enumerates every pipeline phase.
var AllPhases = []Phase{PhasePlan, PhaseExecute, PhaseReview, PhaseGate}
