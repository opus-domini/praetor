package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/orchestration/fsm"
)

type iterationStateFn func(ctx context.Context, machine *iterationMachine) (iterationStateFn, error)

type iterationMachine struct {
	runner *Runner
	run    *activeRun
	next   iterationStateFn

	stop bool

	selected       taskSelection
	runID          string
	taskWorkdir    string
	executorOutput string

	outcome       taskOutcome
	hasOutcome    bool
	persistReason string
	persistTaskID string
}

func iterationStep(ctx context.Context, machine *iterationMachine) (fsm.StateFn[iterationMachine], error) {
	if machine == nil || machine.next == nil {
		return nil, nil
	}
	next, err := machine.next(ctx, machine)
	if err != nil {
		return nil, err
	}
	machine.next = next
	if machine.next == nil {
		return nil, nil
	}
	return iterationStep, nil
}

func iterationStateSelectTask(ctx context.Context, machine *iterationMachine) (iterationStateFn, error) {
	if err := machine.persistBoundary("selectTask", "selecting runnable task", ""); err != nil {
		return nil, err
	}

	run := machine.run
	index, task, ok := domain.NextRunnableTask(run.state)
	if !ok {
		if run.state.OpenCount() == 0 {
			_ = run.appendSnapshotEvent("completed", "", "all tasks completed")
			_ = run.persistSnapshot("completed", "all tasks completed")
			run.render.Success("All tasks completed")
			machine.stop = true
			return nil, nil
		}

		report := domain.BlockedTasksReport(run.state, 5)
		if len(report) == 0 {
			if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "blocked",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   "plan is blocked: open tasks exist but none are runnable",
			}); err != nil {
				return nil, fmt.Errorf("write blocked checkpoint: %w", err)
			}
			_ = run.appendSnapshotEvent("blocked", "", "plan is blocked: open tasks exist but none are runnable")
			_ = run.persistSnapshot("blocked", "plan is blocked: open tasks exist but none are runnable")
			return nil, errors.New("plan is blocked: open tasks exist but none are runnable")
		}
		if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "blocked",
			TaskID:    "",
			Signature: "",
			RunID:     "",
			Message:   fmt.Sprintf("plan is blocked by dependencies: %s", strings.Join(report, ", ")),
		}); err != nil {
			return nil, fmt.Errorf("write blocked checkpoint: %w", err)
		}
		_ = run.appendSnapshotEvent("blocked", "", "plan is blocked by dependencies")
		_ = run.persistSnapshot("blocked", "plan is blocked by dependencies")
		return nil, fmt.Errorf("plan is blocked by dependencies:\n- %s", strings.Join(report, "\n- "))
	}

	executor, err := resolveExecutor(task, run.options.DefaultExecutor)
	if err != nil {
		return nil, fmt.Errorf("task %s: %w", task.ID, err)
	}
	reviewer, err := resolveReviewer(task, run.options.DefaultReviewer, run.options.SkipReview)
	if err != nil {
		return nil, fmt.Errorf("task %s: %w", task.ID, err)
	}

	signature := run.store.TaskSignatureForPlan(run.planFile, index, task)
	if task.Attempt >= run.options.MaxRetries {
		return nil, fmt.Errorf("retry limit reached for task %s (%s)", task.ID, task.Title)
	}

	progress := fmt.Sprintf("%d/%d", run.state.DoneCount()+1, len(run.state.Tasks))
	selected := taskSelection{
		index:     index,
		task:      task,
		executor:  executor,
		reviewer:  reviewer,
		signature: signature,
		retries:   task.Attempt,
		feedback:  task.Feedback,
		progress:  progress,
	}

	runDir, err := prepareRunDir(run.store.LogsDir(), task, signature)
	if err != nil {
		return nil, err
	}
	runID := filepath.Base(runDir)
	taskWorkdir, err := run.isolation.PrepareTask(ctx, runID, task.ID)
	if err != nil {
		return nil, err
	}

	taskLabel := task.ID
	if strings.TrimSpace(taskLabel) == "" {
		taskLabel = fmt.Sprintf("#%d", index)
	}
	selected.taskLabel = taskLabel
	machine.selected = selected
	machine.runID = runID
	machine.taskWorkdir = taskWorkdir
	machine.persistTaskID = task.ID

	_ = run.appendSnapshotEvent("task_selected", task.ID, fmt.Sprintf("task selected: %s", task.Title))
	_ = run.persistSnapshot("task_selected", fmt.Sprintf("task selected: %s", task.ID))
	run.render.Task(progress, taskLabel, task.Title)

	if err := run.transitions.TransitionTask(&run.state, index, domain.TaskExecuting, run.planFile); err != nil {
		return nil, err
	}
	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d", selected.retries+1, run.options.MaxRetries))

	return iterationStateExecuteTask, nil
}

func iterationStateExecuteTask(ctx context.Context, machine *iterationMachine) (iterationStateFn, error) {
	if err := machine.persistBoundary("executeTask", "executing task", machine.selected.task.ID); err != nil {
		return nil, err
	}

	run := machine.run
	selected := machine.selected
	runDir := filepath.Join(run.store.LogsDir(), machine.runID)

	executorSystemPrompt := BuildExecutorSystemPrompt(run.projectContext)
	executorTaskPrompt := BuildExecutorTaskPrompt(run.planFile, selected.index, selected.task, selected.feedback, selected.retries, run.plan.Title, selected.progress, machine.taskWorkdir)
	_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
	_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

	execResult, execErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:         "execute",
		Agent:        selected.executor,
		Prompt:       executorTaskPrompt,
		SystemPrompt: executorSystemPrompt,
		Model:        selected.task.Model,
		Workdir:      machine.taskWorkdir,
		RunDir:       runDir,
		OutputPrefix: "executor",
		TaskLabel:    selected.taskLabel,
		CodexBin:     run.options.CodexBin,
		ClaudeBin:    run.options.ClaudeBin,
		Verbose:      run.options.Verbose,
	})
	machine.executorOutput = execResult.Output
	run.totalCost += execResult.CostUSD
	_ = writeText(filepath.Join(runDir, "executor.output.txt"), machine.executorOutput)
	logRuntimeStrategy(run, selected.task.ID, selected.signature, machine.runID, "executor", execResult.Strategy)

	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d [%.1fs]", selected.retries+1, run.options.MaxRetries, execResult.DurationS))

	if execErr != nil {
		if isCancellationErr(execErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, execErr)
			machine.setOutcome(taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr})
			return iterationStateApplyOutcome, nil
		}
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "executor_crashed",
			message:      fmt.Sprintf("executor process failed: %v", execErr),
			feedback:     fmt.Sprintf("executor process failed: %v", execErr),
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Executor failed: %v (retry %d/%d)",
			renderArgs:   []any{execErr},
			metrics: []domain.CostEntry{{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     machine.runID,
				TaskID:    selected.task.ID,
				Agent:     string(selected.executor),
				Role:      "executor",
				DurationS: execResult.DurationS,
				Status:    "fail",
				CostUSD:   execResult.CostUSD,
			}},
		})
		return iterationStateApplyOutcome, nil
	}

	result := domain.ParseExecutorResult(machine.executorOutput)
	if result != domain.ExecutorResultPass {
		feedback := "executor self-reported RESULT: FAIL"
		if result == domain.ExecutorResultUnknown {
			feedback = "executor output missing or invalid RESULT line"
		}
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "executor_fail",
			message:      fmt.Sprintf("executor reported RESULT: %s", result),
			feedback:     feedback,
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Executor reported %s (retry %d/%d)",
			renderArgs:   []any{result},
			metrics: []domain.CostEntry{{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     machine.runID,
				TaskID:    selected.task.ID,
				Agent:     string(selected.executor),
				Role:      "executor",
				DurationS: execResult.DurationS,
				Status:    "fail",
				CostUSD:   execResult.CostUSD,
			}},
		})
		return iterationStateApplyOutcome, nil
	}

	if err := run.transitions.WriteMetric(domain.CostEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     machine.runID,
		TaskID:    selected.task.ID,
		Agent:     string(selected.executor),
		Role:      "executor",
		DurationS: execResult.DurationS,
		Status:    "pass",
		CostUSD:   execResult.CostUSD,
	}); err != nil {
		return nil, fmt.Errorf("write executor metric: %w", err)
	}

	if run.options.PostTaskHook != "" {
		run.render.Phase("hook", "post-task", run.options.PostTaskHook)
		hookPassed, hookFeedback := runPostTaskHook(ctx, run.options.PostTaskHook, machine.taskWorkdir, runDir)
		if !hookPassed {
			if ctx.Err() != nil {
				cancelErr := cancellationCause(ctx, nil)
				machine.setOutcome(taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr})
				return iterationStateApplyOutcome, nil
			}
			machine.setOutcome(taskOutcome{
				kind:         taskOutcomeRetry,
				status:       "hook_failed",
				message:      "post-task hook failed",
				feedback:     hookFeedback,
				rollback:     true,
				renderLevel:  "error",
				renderFormat: "Post-task hook failed (retry %d/%d)",
			})
			return iterationStateApplyOutcome, nil
		}
	}

	if selected.reviewer == domain.AgentNone {
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeComplete,
			message:      fmt.Sprintf("task completed: %s", selected.task.Title),
			renderFormat: "Completed: %s",
			renderArgs:   []any{selected.taskLabel},
		})
		return iterationStateApplyOutcome, nil
	}

	if err := run.transitions.TransitionTask(&run.state, selected.index, domain.TaskReviewing, run.planFile); err != nil {
		return nil, err
	}
	run.render.Phase("reviewer", string(selected.reviewer), "reviewing task result")
	return iterationStateReviewTask, nil
}

func iterationStateReviewTask(ctx context.Context, machine *iterationMachine) (iterationStateFn, error) {
	if err := machine.persistBoundary("reviewTask", "reviewing task result", machine.selected.task.ID); err != nil {
		return nil, err
	}

	run := machine.run
	selected := machine.selected
	runDir := filepath.Join(run.store.LogsDir(), machine.runID)
	gitDiff := CaptureGitDiff(machine.taskWorkdir, 500)

	reviewerSystemPrompt := BuildReviewerSystemPrompt(run.projectContext)
	reviewerTaskPrompt := BuildReviewerTaskPrompt(run.planFile, selected.task, machine.executorOutput, machine.taskWorkdir, run.plan.Title, selected.progress, gitDiff)
	_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
	_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

	reviewResult, reviewErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:         "review",
		Agent:        selected.reviewer,
		Prompt:       reviewerTaskPrompt,
		SystemPrompt: reviewerSystemPrompt,
		Model:        selected.task.Model,
		Workdir:      machine.taskWorkdir,
		RunDir:       runDir,
		OutputPrefix: "reviewer",
		TaskLabel:    selected.taskLabel,
		CodexBin:     run.options.CodexBin,
		ClaudeBin:    run.options.ClaudeBin,
		Verbose:      run.options.Verbose,
	})
	reviewerOutput := reviewResult.Output
	run.totalCost += reviewResult.CostUSD
	_ = writeText(filepath.Join(runDir, "reviewer.output.txt"), reviewerOutput)
	logRuntimeStrategy(run, selected.task.ID, selected.signature, machine.runID, "reviewer", reviewResult.Strategy)

	run.render.Phase("reviewer", string(selected.reviewer), fmt.Sprintf("review complete [%.1fs]", reviewResult.DurationS))

	if reviewErr != nil {
		if isCancellationErr(reviewErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, reviewErr)
			machine.setOutcome(taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr})
			return iterationStateApplyOutcome, nil
		}
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "reviewer_crashed",
			message:      fmt.Sprintf("reviewer process failed: %v", reviewErr),
			feedback:     fmt.Sprintf("reviewer process failed: %v", reviewErr),
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Reviewer failed: %v (retry %d/%d)",
			renderArgs:   []any{reviewErr},
			metrics: []domain.CostEntry{{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     machine.runID,
				TaskID:    selected.task.ID,
				Agent:     string(selected.reviewer),
				Role:      "reviewer",
				DurationS: reviewResult.DurationS,
				Status:    "fail",
				CostUSD:   reviewResult.CostUSD,
			}},
		})
		return iterationStateApplyOutcome, nil
	}

	if err := run.transitions.WriteMetric(domain.CostEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     machine.runID,
		TaskID:    selected.task.ID,
		Agent:     string(selected.reviewer),
		Role:      "reviewer",
		DurationS: reviewResult.DurationS,
		Status:    "pass",
		CostUSD:   reviewResult.CostUSD,
	}); err != nil {
		return nil, fmt.Errorf("write reviewer metric: %w", err)
	}

	decision := domain.ParseReviewDecision(reviewerOutput)
	if !decision.Pass {
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "review_rejected",
			message:      decision.Reason,
			feedback:     decision.Reason,
			rollback:     true,
			renderLevel:  "warn",
			renderFormat: "Review rejected: %s (retry %d/%d)",
			renderArgs:   []any{decision.Reason},
		})
		return iterationStateApplyOutcome, nil
	}

	machine.setOutcome(taskOutcome{
		kind:         taskOutcomeComplete,
		message:      fmt.Sprintf("task completed: %s", selected.task.Title),
		renderFormat: "Completed: %s",
		renderArgs:   []any{selected.taskLabel},
	})
	return iterationStateApplyOutcome, nil
}

func iterationStateApplyOutcome(ctx context.Context, machine *iterationMachine) (iterationStateFn, error) {
	if err := machine.persistBoundary("applyOutcome", "applying task outcome", machine.selected.task.ID); err != nil {
		return nil, err
	}
	if !machine.hasOutcome {
		return nil, errors.New("missing iteration outcome")
	}

	stop, err := machine.runner.applyTaskOutcome(ctx, machine.run, machine.selected, machine.runID, machine.outcome)
	if err != nil {
		return nil, err
	}
	machine.stop = stop
	machine.persistReason = machine.outcome.message
	machine.hasOutcome = false
	return iterationStatePersist, nil
}

func iterationStatePersist(_ context.Context, machine *iterationMachine) (iterationStateFn, error) {
	message := strings.TrimSpace(machine.persistReason)
	if message == "" {
		message = "iteration persisted"
	}
	if err := machine.persistBoundary("persist", message, machine.persistTaskID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (machine *iterationMachine) setOutcome(outcome taskOutcome) {
	machine.outcome = outcome
	machine.hasOutcome = true
}

func (machine *iterationMachine) persistBoundary(phase, message, taskID string) error {
	if machine == nil || machine.run == nil {
		return nil
	}
	if err := machine.run.persistSnapshot(phase, message); err != nil {
		return err
	}
	_ = machine.run.appendSnapshotEvent("state", strings.TrimSpace(taskID), fmt.Sprintf("%s: %s", strings.TrimSpace(phase), strings.TrimSpace(message)))
	return nil
}

func logRuntimeStrategy(run *activeRun, taskID, signature, runID, role string, strategy domain.ExecutionStrategy) {
	strategyName := strings.TrimSpace(string(strategy))
	if strategyName == "" {
		return
	}
	msg := fmt.Sprintf("%s strategy: %s", strings.TrimSpace(role), strategyName)
	_ = run.appendSnapshotEvent("runtime_strategy", taskID, msg)
	_ = run.transitions.WriteCheckpoint(domain.CheckpointEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    "runtime_strategy",
		TaskID:    taskID,
		Signature: signature,
		RunID:     runID,
		Message:   msg,
	})
}
