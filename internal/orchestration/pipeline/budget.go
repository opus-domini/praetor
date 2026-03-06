package pipeline

import (
	"fmt"
	"math"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

const microsPerUSD int64 = 1_000_000
const microsPerCent int64 = 10_000

// CostAccumulator tracks observed run spend and evaluates budget policy.
type CostAccumulator struct {
	policy         domain.CostPolicy
	planTotal      int64
	taskCosts      map[string]int64
	warningEmitted bool
}

func NewCostAccumulator(policy domain.CostPolicy) *CostAccumulator {
	return &CostAccumulator{
		policy:    policy,
		taskCosts: make(map[string]int64),
	}
}

func (c *CostAccumulator) Seed(planTotal int64, taskCosts map[string]int64, warningEmitted bool) {
	if c == nil {
		return
	}
	c.planTotal = maxInt64(planTotal, 0)
	c.warningEmitted = warningEmitted
	clear(c.taskCosts)
	for taskID, total := range taskCosts {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			continue
		}
		c.taskCosts[taskID] = maxInt64(total, 0)
	}
}

func (c *CostAccumulator) Record(taskID string, costUSD float64) int64 {
	if c == nil {
		return 0
	}
	delta := costUSDToMicros(costUSD)
	if delta <= 0 {
		return 0
	}
	c.planTotal += delta
	taskID = strings.TrimSpace(taskID)
	if taskID != "" {
		c.taskCosts[taskID] += delta
	}
	return delta
}

func (c *CostAccumulator) PlanTotalMicros() int64 {
	if c == nil {
		return 0
	}
	return c.planTotal
}

func (c *CostAccumulator) TaskCostMicros(taskID string) int64 {
	if c == nil {
		return 0
	}
	return c.taskCosts[strings.TrimSpace(taskID)]
}

func (c *CostAccumulator) IsOverPlanBudget() bool {
	if c == nil || c.policy.PlanLimitCents <= 0 {
		return false
	}
	return c.planTotal > centsToMicros(c.policy.PlanLimitCents)
}

func (c *CostAccumulator) IsOverTaskBudget(taskID string) bool {
	if c == nil || c.policy.TaskLimitCents <= 0 {
		return false
	}
	return c.TaskCostMicros(taskID) > centsToMicros(c.policy.TaskLimitCents)
}

func (c *CostAccumulator) IsWarning() bool {
	if c == nil || c.warningEmitted || c.policy.PlanLimitCents <= 0 {
		return false
	}
	threshold := c.policy.WarnThreshold
	if threshold <= 0 {
		return false
	}
	return float64(c.planTotal) >= float64(centsToMicros(c.policy.PlanLimitCents))*threshold
}

func (c *CostAccumulator) WarningMessage() string {
	if c == nil || c.policy.PlanLimitCents <= 0 {
		return ""
	}
	percent := 0.0
	limit := centsToMicros(c.policy.PlanLimitCents)
	if limit > 0 {
		percent = (float64(c.planTotal) / float64(limit)) * 100
	}
	return fmt.Sprintf("cost budget warning: %s / %s (%.1f%%)",
		formatUSDFromMicros(c.planTotal),
		formatUSDFromMicros(limit),
		percent,
	)
}

func (c *CostAccumulator) MarkWarningEmitted() {
	if c == nil {
		return
	}
	c.warningEmitted = true
}

func (c *CostAccumulator) WarningEmitted() bool {
	if c == nil {
		return false
	}
	return c.warningEmitted
}

func costUSDToMicros(costUSD float64) int64 {
	if costUSD <= 0 {
		return 0
	}
	return int64(math.Round(costUSD * float64(microsPerUSD)))
}

func microsToUSD(micros int64) float64 {
	if micros == 0 {
		return 0
	}
	return float64(micros) / float64(microsPerUSD)
}

func centsToMicros(cents int64) int64 {
	if cents <= 0 {
		return 0
	}
	return cents * microsPerCent
}

func formatUSDFromMicros(micros int64) string {
	return fmt.Sprintf("$%.4f", microsToUSD(micros))
}

type budgetExceededError struct {
	scope       string
	taskID      string
	totalMicros int64
	limitMicros int64
}

func (e *budgetExceededError) Error() string {
	if e == nil {
		return "cost budget exceeded"
	}
	switch strings.TrimSpace(e.scope) {
	case "task":
		if strings.TrimSpace(e.taskID) != "" {
			return fmt.Sprintf("task cost budget exceeded for %s: %s > %s", e.taskID, formatUSDFromMicros(e.totalMicros), formatUSDFromMicros(e.limitMicros))
		}
		return fmt.Sprintf("task cost budget exceeded: %s > %s", formatUSDFromMicros(e.totalMicros), formatUSDFromMicros(e.limitMicros))
	default:
		return fmt.Sprintf("plan cost budget exceeded: %s > %s", formatUSDFromMicros(e.totalMicros), formatUSDFromMicros(e.limitMicros))
	}
}

func (e *budgetExceededError) Is(target error) bool {
	_, ok := target.(*budgetExceededError)
	return ok
}

func newPlanBudgetExceededError(totalMicros, limitMicros int64) error {
	return &budgetExceededError{
		scope:       "plan",
		totalMicros: totalMicros,
		limitMicros: limitMicros,
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
