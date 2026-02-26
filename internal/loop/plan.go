package loop

import (
	"time"

	"github.com/opus-domini/praetor/internal/domain"
)

// LoadPlan delegates to domain.LoadPlan.
func LoadPlan(path string) (Plan, error) {
	return domain.LoadPlan(path)
}

// ValidatePlan delegates to domain.ValidatePlan.
func ValidatePlan(plan Plan) error {
	return domain.ValidatePlan(plan)
}

// PlanChecksum delegates to domain.PlanChecksum.
func PlanChecksum(path string) (string, error) {
	return domain.PlanChecksum(path)
}

// NewPlanFile delegates to domain.NewPlanFile.
func NewPlanFile(slug string, now time.Time, baseDir string) (string, error) {
	return domain.NewPlanFile(slug, now, baseDir)
}

// canonicalTaskID delegates to domain.CanonicalTaskID.
func canonicalTaskID(task Task, index int) string {
	return domain.CanonicalTaskID(task, index)
}

// autoTaskFingerprint delegates to domain.AutoTaskFingerprint.
func autoTaskFingerprint(title string, executor Agent, reviewer Agent, model, description, criteria string, dependsOn []string) string {
	return domain.AutoTaskFingerprint(title, executor, reviewer, model, description, criteria, dependsOn)
}
