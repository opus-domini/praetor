package loop

import (
	"context"
	"fmt"
	"time"
)

type taskSelection struct {
	index     int
	task      StateTask
	executor  Agent
	reviewer  Agent
	signature string
	retries   int
	feedback  string
	progress  string
	taskLabel string
}

type taskOutcomeKind string

const (
	taskOutcomeRetry    taskOutcomeKind = "retry"
	taskOutcomeComplete taskOutcomeKind = "complete"
	taskOutcomeCanceled taskOutcomeKind = "canceled"
)

type taskOutcome struct {
	kind         taskOutcomeKind
	status       string
	message      string
	feedback     string
	metrics      []CostEntry
	rollback     bool
	renderLevel  string
	renderFormat string
	renderArgs   []any
	cancelErr    error
}

func (r *Runner) applyTaskOutcome(ctx context.Context, run *activeRun, selected taskSelection, runID string, outcome taskOutcome) (bool, error) {
	// All task-side transitions flow through this function so retries, metrics,
	// checkpoints, and user-visible status stay consistent across branches.
	for _, metric := range outcome.metrics {
		if err := run.transitions.WriteMetric(metric); err != nil {
			return false, err
		}
	}

	switch outcome.kind {
	case taskOutcomeCanceled:
		if err := run.transitions.WriteCheckpoint(CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "canceled",
			TaskID:    selected.task.ID,
			Signature: selected.signature,
			RunID:     runID,
			Message:   outcome.message,
		}); err != nil {
			return false, err
		}
		return false, outcome.cancelErr

	case taskOutcomeRetry:
		run.stats.TasksRejected++
		nextRetry, err := run.transitions.RetryTask(selected.signature, outcome.feedback)
		if err != nil {
			return false, err
		}
		if outcome.rollback {
			run.isolation.RollbackTask(context.WithoutCancel(ctx), runID, run.render)
		}
		if err := run.transitions.WriteCheckpoint(CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    outcome.status,
			TaskID:    selected.task.ID,
			Signature: selected.signature,
			RunID:     runID,
			Message:   outcome.message,
		}); err != nil {
			return false, err
		}

		renderArgs := append([]any{}, outcome.renderArgs...)
		renderArgs = append(renderArgs, nextRetry, run.options.MaxRetries)
		renderMsg := fmt.Sprintf(outcome.renderFormat, renderArgs...)
		switch outcome.renderLevel {
		case "warn":
			run.render.Warn(renderMsg)
		default:
			run.render.Error(renderMsg)
		}
		run.stats.Iterations++
		return false, nil

	case taskOutcomeComplete:
		if err := run.isolation.CommitTask(ctx, runID); err != nil {
			return false, err
		}
		if err := run.transitions.CompleteTask(&run.state, selected.index, selected.signature, runID, outcome.message); err != nil {
			return false, err
		}
		run.stats.TasksDone++
		run.stats.Iterations++
		run.render.Success(fmt.Sprintf(outcome.renderFormat, outcome.renderArgs...))
		return false, nil
	}

	return false, fmt.Errorf("unsupported task outcome kind %q", outcome.kind)
}
