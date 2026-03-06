package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
)

func (run *activeRun) recordCostMetric(taskIndex int, entry domain.CostEntry) error {
	if run == nil {
		return nil
	}
	if strings.TrimSpace(entry.Timestamp) == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	entry.PlanSlug = run.slug
	if err := run.transitions.WriteMetric(entry); err != nil {
		return err
	}
	if run.costBudget == nil {
		return nil
	}

	run.costBudget.Record(entry.TaskID, entry.CostUSD)
	run.state.TotalCostMicros = run.costBudget.PlanTotalMicros()
	if taskIndex >= 0 && taskIndex < len(run.state.Tasks) {
		run.state.Tasks[taskIndex].CostMicros = run.costBudget.TaskCostMicros(entry.TaskID)
	}
	run.recordActorCall(domain.EventActor{Role: entry.Role, Agent: entry.Agent}, entry.CostUSD, entry.DurationS)
	run.totalCost = microsToUSD(run.state.TotalCostMicros)

	if err := run.store.WriteState(run.slug, run.state); err != nil {
		return err
	}
	return run.emitCostWarningIfNeeded()
}

func (run *activeRun) emitCostWarningIfNeeded() error {
	if run == nil || run.costBudget == nil || !run.costBudget.IsWarning() {
		return nil
	}
	run.costBudget.MarkWarningEmitted()
	run.state.CostWarningEmitted = true
	if err := run.store.WriteState(run.slug, run.state); err != nil {
		return err
	}

	message := run.costBudget.WarningMessage()
	run.eventSink.Emit(middleware.ExecutionEvent{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Type:          middleware.EventCostBudgetWarning,
		EventType:     string(middleware.EventCostBudgetWarning),
		RunID:         run.runID,
		Message:       message,
		Actor:         &domain.EventActor{Role: "system", Agent: "cost-budget"},
		CostUSD:       microsToUSD(run.state.TotalCostMicros),
		Data: map[string]any{
			"scope":               "plan",
			"plan_cost_micros":    run.state.TotalCostMicros,
			"plan_limit_cents":    run.state.ExecutionPolicy.Cost.PlanLimitCents,
			"task_limit_cents":    run.state.ExecutionPolicy.Cost.TaskLimitCents,
			"warn_threshold":      run.state.ExecutionPolicy.Cost.WarnThreshold,
			"cost_budget_enforce": costBudgetEnforced(run.state.ExecutionPolicy.Cost),
		},
	})
	_ = run.appendSnapshotEvent("cost_budget_warning", "", message)
	return nil
}

func (run *activeRun) planBudgetExceededError() error {
	if run == nil || run.costBudget == nil || !costBudgetEnforced(run.state.ExecutionPolicy.Cost) || !run.costBudget.IsOverPlanBudget() {
		return nil
	}
	totalMicros := run.costBudget.PlanTotalMicros()
	limitMicros := centsToMicros(run.state.ExecutionPolicy.Cost.PlanLimitCents)
	err := newPlanBudgetExceededError(totalMicros, limitMicros)
	run.emitCostBudgetExceededEvent("plan", "", totalMicros, limitMicros, err.Error())
	return err
}

func (run *activeRun) taskBudgetExceededError(taskID string) error {
	if run == nil || run.costBudget == nil || !costBudgetEnforced(run.state.ExecutionPolicy.Cost) || !run.costBudget.IsOverTaskBudget(taskID) {
		return nil
	}
	totalMicros := run.costBudget.TaskCostMicros(taskID)
	limitMicros := centsToMicros(run.state.ExecutionPolicy.Cost.TaskLimitCents)
	err := &budgetExceededError{
		scope:       "task",
		taskID:      strings.TrimSpace(taskID),
		totalMicros: totalMicros,
		limitMicros: limitMicros,
	}
	run.emitCostBudgetExceededEvent("task", taskID, totalMicros, limitMicros, err.Error())
	return err
}

func (run *activeRun) emitCostBudgetExceededEvent(scope, taskID string, totalMicros, limitMicros int64, message string) {
	if run == nil {
		return
	}
	if run.eventSink != nil {
		run.eventSink.Emit(middleware.ExecutionEvent{
			SchemaVersion: 1,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Type:          middleware.EventCostBudgetExceeded,
			EventType:     string(middleware.EventCostBudgetExceeded),
			RunID:         run.runID,
			TaskID:        strings.TrimSpace(taskID),
			Message:       strings.TrimSpace(message),
			Actor:         &domain.EventActor{Role: "system", Agent: "cost-budget"},
			CostUSD:       microsToUSD(totalMicros),
			Data: map[string]any{
				"scope":               strings.TrimSpace(scope),
				"task_id":             strings.TrimSpace(taskID),
				"total_cost_micros":   totalMicros,
				"limit_micros":        limitMicros,
				"cost_budget_enforce": costBudgetEnforced(run.state.ExecutionPolicy.Cost),
			},
		})
	}
	_ = run.appendSnapshotEvent("cost_budget_exceeded", strings.TrimSpace(taskID), strings.TrimSpace(message))
}

func newTaskBudgetExceededOutcome(taskID string, attempt int, totalMicros, limitMicros int64) taskOutcome {
	message := formatTaskBudgetExceeded(taskID, totalMicros, limitMicros)
	actor := &domain.EventActor{Role: "system", Agent: "cost-budget"}
	return taskOutcome{
		kind:     taskOutcomeRetry,
		status:   "task_cost_budget_exceeded",
		message:  message,
		feedback: message,
		structuredFeedback: buildTaskFeedback(
			taskID,
			attempt,
			"cost",
			*actor,
			"fail",
			message,
			[]string{"Reduce task scope or raise the task budget before retrying."},
			"",
		),
		actor:        actor,
		rollback:     true,
		forceFailed:  true,
		renderLevel:  "warn",
		renderFormat: "Task cost budget exceeded: %s",
		renderArgs:   []any{strings.TrimSpace(taskID)},
	}
}

func costBudgetEnforced(policy domain.CostPolicy) bool {
	return policy.Enforce == nil || *policy.Enforce
}

func formatTaskBudgetExceeded(taskID string, totalMicros, limitMicros int64) string {
	if strings.TrimSpace(taskID) == "" {
		return fmt.Sprintf("task cost budget exceeded: %s > %s", formatUSDFromMicros(totalMicros), formatUSDFromMicros(limitMicros))
	}
	return fmt.Sprintf("task cost budget exceeded for %s: %s > %s", strings.TrimSpace(taskID), formatUSDFromMicros(totalMicros), formatUSDFromMicros(limitMicros))
}
