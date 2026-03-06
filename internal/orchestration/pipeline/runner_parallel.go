package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
)

type preparedTask struct {
	selection   taskSelection
	runID       string
	taskWorkdir string
}

type taskWorkerResult struct {
	prepared preparedTask
	outcome  taskOutcome
	err      error
}

func (r *Runner) runParallelWave(ctx context.Context, run *activeRun) (bool, error) {
	prepared, stop, err := r.prepareWave(ctx, run)
	if err != nil || stop {
		return stop, err
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultsCh := make(chan taskWorkerResult, len(prepared))
	for _, item := range prepared {
		item := item
		go func() {
			outcome, execErr := r.executePreparedTask(workerCtx, run, item)
			if execErr != nil {
				cancel()
			}
			resultsCh <- taskWorkerResult{
				prepared: item,
				outcome:  outcome,
				err:      execErr,
			}
		}()
	}

	results := make([]taskWorkerResult, 0, len(prepared))
	for range prepared {
		results = append(results, <-resultsCh)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].prepared.selection.index < results[j].prepared.selection.index
	})

	for _, result := range results {
		if result.err != nil {
			run.isolation.RollbackTask(context.WithoutCancel(ctx), result.prepared.runID, run.render)
			_ = run.store.ReleaseTaskLock(result.prepared.selection.taskLock)
			return false, result.err
		}
		if _, err := r.applyTaskOutcomeWithBudgetPolicy(ctx, run, result.prepared.selection, result.prepared.runID, result.outcome, true); err != nil {
			if result.outcome.kind == taskOutcomeComplete && isMergeConflictError(err) {
				if conflictErr := requeueParallelConflict(run, result.prepared.selection, result.prepared.runID, err); conflictErr != nil {
					return false, conflictErr
				}
				continue
			}
			return false, err
		}
		if result.outcome.kind == taskOutcomeComplete {
			emitParallelMergeEvent(run, result.prepared.selection.task.ID, "merged", "parallel wave merge applied")
		}
	}

	if err := run.planBudgetExceededError(); err != nil {
		return false, err
	}
	return false, nil
}

func (r *Runner) prepareWave(ctx context.Context, run *activeRun) ([]preparedTask, bool, error) {
	indices := domain.RunnableTaskIndices(run.state, run.options.MaxParallelTasks)
	if len(indices) == 0 {
		stop, err := handleNoRunnableTasks(run)
		return nil, stop, err
	}

	prepared := make([]preparedTask, 0, len(indices))
	for offset, index := range indices {
		item, err := r.prepareSelectedTask(ctx, run, index, offset)
		if err != nil {
			return nil, false, err
		}
		prepared = append(prepared, item)
	}
	return prepared, false, nil
}

func handleNoRunnableTasks(run *activeRun) (bool, error) {
	if run == nil {
		return false, nil
	}
	if run.state.ActiveCount() == 0 {
		if run.state.FailedCount() > 0 {
			message := fmt.Sprintf("run completed with %d failed task(s)", run.state.FailedCount())
			_ = run.appendSnapshotEvent("completed_partial", "", message)
			_ = run.persistSnapshot("completed_partial", message)
			run.render.Warn(message)
		} else {
			_ = run.appendSnapshotEvent("completed", "", "all tasks completed")
			_ = run.persistSnapshot("completed", "all tasks completed")
			run.render.Success("All tasks completed")
		}
		return true, nil
	}

	report := domain.BlockedTasksReport(run.state, 5)
	if len(report) == 0 {
		message := "plan is blocked: open tasks exist but none are runnable"
		if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "blocked",
			TaskID:    "",
			Signature: "",
			RunID:     "",
			Message:   message,
		}); err != nil {
			return false, fmt.Errorf("write blocked checkpoint: %w", err)
		}
		_ = run.appendSnapshotEvent("blocked", "", message)
		_ = run.persistSnapshot("blocked", message)
		return false, errors.New(message)
	}

	message := fmt.Sprintf("plan is blocked by dependencies: %s", strings.Join(report, ", "))
	if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    "blocked",
		TaskID:    "",
		Signature: "",
		RunID:     "",
		Message:   message,
	}); err != nil {
		return false, fmt.Errorf("write blocked checkpoint: %w", err)
	}
	_ = run.appendSnapshotEvent("blocked", "", "plan is blocked by dependencies")
	_ = run.persistSnapshot("blocked", "plan is blocked by dependencies")
	return false, fmt.Errorf("plan is blocked by dependencies:\n- %s", strings.Join(report, "\n- "))
}

func (r *Runner) prepareSelectedTask(ctx context.Context, run *activeRun, index, waveOffset int) (preparedTask, error) {
	task := run.state.Tasks[index]

	executorAgent := run.options.DefaultExecutor
	reviewerAgent := run.options.DefaultReviewer
	if index < len(run.plan.Tasks) {
		planTask := run.plan.Tasks[index]
		if planTask.Agents != nil {
			if planTask.Agents.Executor != "" {
				executorAgent = domain.NormalizeAgent(domain.Agent(planTask.Agents.Executor))
			}
			if planTask.Agents.Reviewer != "" {
				reviewerAgent = domain.NormalizeAgent(domain.Agent(planTask.Agents.Reviewer))
			}
		}
	}
	executor, err := resolveExecutorWithRouting(executorAgent, run.availableAgents)
	if err != nil {
		return preparedTask{}, fmt.Errorf("task %s: %w", task.ID, err)
	}
	reviewer, err := resolveReviewer(reviewerAgent, run.options.SkipReview)
	if err != nil {
		return preparedTask{}, fmt.Errorf("task %s: %w", task.ID, err)
	}

	signature := run.store.TaskSignatureForPlan(run.slug, index, task)
	if task.Attempt >= run.options.MaxRetries {
		return preparedTask{}, fmt.Errorf("retry limit reached for task %s (%s)", task.ID, task.Title)
	}

	progress := fmt.Sprintf("%d/%d", run.state.DoneCount()+waveOffset+1, len(run.state.Tasks))
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
		return preparedTask{}, err
	}
	runID := filepath.Base(runDir)

	taskLock, err := run.store.AcquireTaskLock(run.slug, task.ID, run.options.Force)
	if err != nil {
		return preparedTask{}, fmt.Errorf("acquire task lock for %s: %w", task.ID, err)
	}
	selected.taskLock = taskLock

	taskWorkdir, err := run.isolation.PrepareTask(ctx, runID, task.ID)
	if err != nil {
		_ = run.store.ReleaseTaskLock(taskLock)
		return preparedTask{}, err
	}

	taskLabel := task.ID
	if strings.TrimSpace(taskLabel) == "" {
		taskLabel = fmt.Sprintf("#%d", index)
	}
	selected.taskLabel = taskLabel

	if err := run.transitions.TransitionTask(&run.state, index, domain.TaskExecuting); err != nil {
		run.isolation.RollbackTask(context.WithoutCancel(ctx), runID, run.render)
		_ = run.store.ReleaseTaskLock(taskLock)
		return preparedTask{}, err
	}

	_ = run.appendSnapshotEvent("task_selected", task.ID, fmt.Sprintf("task selected: %s", task.Title))
	_ = run.persistSnapshot("task_selected", fmt.Sprintf("task selected: %s", task.ID))
	emitTaskEvent(run, middleware.EventTaskStarted, task.ID, promptPhaseExecute, fmt.Sprintf("task started: %s", task.Title), taskActor("executor", selected.executor, ""), map[string]any{
		"task_id":  task.ID,
		"progress": progress,
	})
	run.render.Task(progress, taskLabel, task.Title)
	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d", selected.retries+1, run.options.MaxRetries))

	return preparedTask{
		selection:   selected,
		runID:       runID,
		taskWorkdir: taskWorkdir,
	}, nil
}

func (r *Runner) executePreparedTask(ctx context.Context, run *activeRun, prepared preparedTask) (taskOutcome, error) {
	selected := prepared.selection
	runDir := filepath.Join(run.store.LogsDir(), prepared.runID)
	metrics := make([]domain.CostEntry, 0, 2)

	var allowedTools, deniedTools []string
	if selected.index < len(run.plan.Tasks) {
		if c := run.plan.Tasks[selected.index].Constraints; c != nil {
			allowedTools = c.AllowedTools
			deniedTools = c.DeniedTools
		}
	}
	executorSystemPrompt := BuildExecutorSystemPrompt(run.promptEngine, run.projectContext, allowedTools, deniedTools)
	feedbackHistory, err := run.store.LoadTaskFeedback(run.slug, selected.signature)
	if err != nil {
		run.render.Warn(fmt.Sprintf("failed to load structured feedback: %v", err))
	}
	if len(feedbackHistory) == 0 && strings.TrimSpace(selected.feedback) != "" {
		feedbackHistory = []domain.TaskFeedback{legacyFeedback(selected.task.ID, selected.retries, selected.feedback)}
	}
	truncatedSections := make([]string, 0)
	if run.promptBudget != nil && len(feedbackHistory) > 0 {
		feedbackHistory, truncatedSections = trimFeedbackHistoryForPrompt(
			run.promptBudget.Budget(promptPhaseExecute),
			run.promptEngine,
			run.slug,
			selected.index,
			selected.task,
			feedbackHistory,
			selected.retries,
			run.plan.Name,
			selected.progress,
			prepared.taskWorkdir,
			run.plan.Quality.Required,
			run.plan.Quality.EvidenceFormat,
		)
	}
	executorTaskPrompt := BuildExecutorTaskPrompt(run.promptEngine, run.slug, selected.index, selected.task, feedbackHistory, selected.retries, run.plan.Name, selected.progress, prepared.taskWorkdir, run.plan.Quality.Required, run.plan.Quality.EvidenceFormat)
	if err := run.appendPerformanceEntry(promptPhaseExecute, executorTaskPrompt, truncatedSections); err != nil {
		run.render.Warn(fmt.Sprintf("failed to write performance metric: %v", err))
	}
	if run.options.Verbose && run.promptBudget != nil {
		run.render.Info(fmt.Sprintf("execute prompt budget: %d/%d chars", len(executorTaskPrompt), run.promptBudget.Budget(promptPhaseExecute)))
	}
	_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
	_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

	executorModel := run.options.ExecutorModel
	if selected.index < len(run.plan.Tasks) {
		if a := run.plan.Tasks[selected.index].Agents; a != nil && a.ExecutorModel != "" {
			executorModel = a.ExecutorModel
		}
	}
	executorActor := taskActor("executor", selected.executor, executorModel)
	execResult, execErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:             "execute",
		Agent:            selected.executor,
		Prompt:           executorTaskPrompt,
		SystemPrompt:     executorSystemPrompt,
		Model:            strings.TrimSpace(executorModel),
		Workdir:          prepared.taskWorkdir,
		RunDir:           runDir,
		OutputPrefix:     "executor",
		TaskLabel:        selected.taskLabel,
		CodexBin:         run.options.CodexBin,
		ClaudeBin:        run.options.ClaudeBin,
		CopilotBin:       run.options.CopilotBin,
		GeminiBin:        run.options.GeminiBin,
		KimiBin:          run.options.KimiBin,
		OpenCodeBin:      run.options.OpenCodeBin,
		OpenRouterURL:    run.options.OpenRouterURL,
		OpenRouterModel:  run.options.OpenRouterModel,
		OpenRouterKeyEnv: run.options.OpenRouterKeyEnv,
		LMStudioURL:      run.options.LMStudioURL,
		LMStudioModel:    run.options.LMStudioModel,
		LMStudioKeyEnv:   run.options.LMStudioKeyEnv,
		Verbose:          run.options.Verbose,
	})
	executorOutput := execResult.Output
	_ = writeText(filepath.Join(runDir, "executor.output.txt"), executorOutput)
	logRuntimeStrategy(run, selected.task.ID, selected.signature, prepared.runID, "executor", execResult.Strategy)
	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d [%.1fs]", selected.retries+1, run.options.MaxRetries, execResult.DurationS))

	if execErr != nil {
		if isCancellationErr(execErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, execErr)
			return taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr, actor: executorActor}, nil //nolint:nilerr // error captured in outcome
		}
		return taskOutcome{
			kind:     taskOutcomeRetry,
			status:   "executor_crashed",
			message:  fmt.Sprintf("executor process failed: %v", execErr),
			feedback: fmt.Sprintf("executor process failed: %v", execErr),
			structuredFeedback: buildTaskFeedback(
				selected.task.ID,
				selected.retries+1,
				promptPhaseExecute,
				*executorActor,
				"fail",
				fmt.Sprintf("executor process failed: %v", execErr),
				[]string{"Investigate the executor failure and rerun the task with a narrower change set."},
				"",
			),
			actor:        executorActor,
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Executor failed: %v (retry %d/%d)",
			renderArgs:   []any{execErr},
			metrics: []domain.CostEntry{{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     prepared.runID,
				TaskID:    selected.task.ID,
				Agent:     string(selected.executor),
				Role:      "executor",
				DurationS: execResult.DurationS,
				Status:    "fail",
				CostUSD:   execResult.CostUSD,
			}},
		}, nil
	}
	if stalledOutcome, stalled := evaluateStallOutcome(run, selected, promptPhaseExecute, executorOutput); stalled {
		return stalledOutcome, nil
	}

	result := domain.ParseExecutorResult(executorOutput)
	if result != domain.ExecutorResultPass {
		feedback := "executor self-reported RESULT: FAIL"
		status := "executor_fail"
		forceFailed := false
		parseClass := string(domain.ParseErrorRecoverable)
		if result == domain.ExecutorResultUnknown {
			feedback = "executor output missing or invalid RESULT line"
			status = "executor_parse_error"
			forceFailed = true
			parseClass = string(domain.ParseErrorNonRecoverable)
		}
		emitParseErrorClassEvent(run, selected.task.ID, promptPhaseExecute, parseClass, feedback)
		return taskOutcome{
			kind:     taskOutcomeRetry,
			status:   status,
			message:  fmt.Sprintf("executor reported RESULT: %s", result),
			feedback: feedback,
			structuredFeedback: buildTaskFeedback(
				selected.task.ID,
				selected.retries+1,
				promptPhaseExecute,
				*executorActor,
				"fail",
				feedback,
				[]string{"Return a valid RESULT block and address the reported executor failure."},
				"",
			),
			actor:        executorActor,
			rollback:     true,
			forceFailed:  forceFailed,
			renderLevel:  "error",
			renderFormat: "Executor reported %s (retry %d/%d)",
			renderArgs:   []any{result},
			metrics: []domain.CostEntry{{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     prepared.runID,
				TaskID:    selected.task.ID,
				Agent:     string(selected.executor),
				Role:      "executor",
				DurationS: execResult.DurationS,
				Status:    "fail",
				CostUSD:   execResult.CostUSD,
			}},
		}, nil
	}

	metrics = append(metrics, domain.CostEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     prepared.runID,
		TaskID:    selected.task.ID,
		Agent:     string(selected.executor),
		Role:      "executor",
		DurationS: execResult.DurationS,
		Status:    "pass",
		CostUSD:   execResult.CostUSD,
	})

	if run.options.PostTaskHook != "" {
		run.render.Phase("hook", "post-task", run.options.PostTaskHook)
		hookPassed, hookFeedback := runPostTaskHook(ctx, run.options.PostTaskHook, prepared.taskWorkdir, runDir)
		if !hookPassed {
			if ctx.Err() != nil {
				cancelErr := cancellationCause(ctx, nil)
				return taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr, actor: executorActor, metrics: metrics}, nil //nolint:nilerr // error captured in outcome
			}
			return taskOutcome{
				kind:     taskOutcomeRetry,
				status:   "hook_failed",
				message:  "post-task hook failed",
				feedback: hookFeedback,
				structuredFeedback: buildTaskFeedback(
					selected.task.ID,
					selected.retries+1,
					"hook",
					domain.EventActor{Role: "system", Agent: "post-task-hook"},
					"fail",
					hookFeedback,
					[]string{"Fix the post-task hook failure before retrying."},
					"",
				),
				actor:        &domain.EventActor{Role: "system", Agent: "post-task-hook"},
				rollback:     true,
				renderLevel:  "error",
				renderFormat: "Post-task hook failed (retry %d/%d)",
				metrics:      metrics,
			}, nil
		}
	}

	if selected.reviewer == domain.AgentNone {
		return taskOutcome{
			kind:         taskOutcomeComplete,
			message:      fmt.Sprintf("task completed: %s", selected.task.Title),
			actor:        executorActor,
			renderFormat: "Completed: %s",
			renderArgs:   []any{selected.taskLabel},
			metrics:      metrics,
		}, nil
	}

	run.render.Phase("reviewer", string(selected.reviewer), "reviewing task result")
	gitDiff := CaptureGitDiff(prepared.taskWorkdir, 0)
	executorOutputForPrompt := executorOutput
	gitDiffForPrompt := gitDiff
	truncatedSections = truncatedSections[:0]
	if run.promptBudget != nil {
		overhead := BuildReviewerTaskPrompt(run.promptEngine, run.slug, selected.task, "", prepared.taskWorkdir, run.plan.Name, selected.progress, "")
		execOut, diff, truncated := run.promptBudget.TruncateReviewSections(overhead, executorOutputForPrompt, gitDiffForPrompt)
		executorOutputForPrompt = execOut
		gitDiffForPrompt = diff
		truncatedSections = append(truncatedSections, truncated...)
	}

	standardsGate := hasGate(run.plan.Quality.Required, "standards")
	reviewerSystemPrompt := BuildReviewerSystemPrompt(run.promptEngine, run.projectContext, standardsGate)
	reviewerTaskPrompt := BuildReviewerTaskPrompt(run.promptEngine, run.slug, selected.task, executorOutputForPrompt, prepared.taskWorkdir, run.plan.Name, selected.progress, gitDiffForPrompt)
	if err := run.appendPerformanceEntry(promptPhaseReview, reviewerTaskPrompt, truncatedSections); err != nil {
		run.render.Warn(fmt.Sprintf("failed to write performance metric: %v", err))
	}
	if run.options.Verbose && run.promptBudget != nil {
		run.render.Info(fmt.Sprintf("review prompt budget: %d/%d chars", len(reviewerTaskPrompt), run.promptBudget.Budget(promptPhaseReview)))
	}
	_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
	_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

	if gateFailure, blocked := executeHostGates(ctx, run, selected.task.ID, prepared.taskWorkdir, executorOutput); blocked {
		return taskOutcome{
			kind:     taskOutcomeRetry,
			status:   "review_rejected",
			message:  gateFailure.reason,
			feedback: gateFailure.reason,
			structuredFeedback: buildTaskFeedback(
				selected.task.ID,
				selected.retries+1,
				string(PhaseGate),
				*gateActor(gateFailure.name),
				"fail",
				gateFailure.reason,
				[]string{"Resolve the failing gate before asking for another review."},
				gateFailure.detail,
			),
			actor:        gateActor(gateFailure.name),
			rollback:     true,
			renderLevel:  "warn",
			renderFormat: "Review rejected: %s (retry %d/%d)",
			renderArgs:   []any{gateFailure.reason},
			metrics:      metrics,
		}, nil
	}

	reviewerModel := run.options.ReviewerModel
	if selected.index < len(run.plan.Tasks) {
		if a := run.plan.Tasks[selected.index].Agents; a != nil && a.ReviewerModel != "" {
			reviewerModel = a.ReviewerModel
		}
	}
	reviewerActor := taskActor("reviewer", selected.reviewer, reviewerModel)
	reviewResult, reviewErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:             "review",
		Agent:            selected.reviewer,
		Prompt:           reviewerTaskPrompt,
		SystemPrompt:     reviewerSystemPrompt,
		Model:            strings.TrimSpace(reviewerModel),
		Workdir:          prepared.taskWorkdir,
		RunDir:           runDir,
		OutputPrefix:     "reviewer",
		TaskLabel:        selected.taskLabel,
		CodexBin:         run.options.CodexBin,
		ClaudeBin:        run.options.ClaudeBin,
		CopilotBin:       run.options.CopilotBin,
		GeminiBin:        run.options.GeminiBin,
		KimiBin:          run.options.KimiBin,
		OpenCodeBin:      run.options.OpenCodeBin,
		OpenRouterURL:    run.options.OpenRouterURL,
		OpenRouterModel:  run.options.OpenRouterModel,
		OpenRouterKeyEnv: run.options.OpenRouterKeyEnv,
		LMStudioURL:      run.options.LMStudioURL,
		LMStudioModel:    run.options.LMStudioModel,
		LMStudioKeyEnv:   run.options.LMStudioKeyEnv,
		Verbose:          run.options.Verbose,
	})
	reviewerOutput := reviewResult.Output
	_ = writeText(filepath.Join(runDir, "reviewer.output.txt"), reviewerOutput)
	logRuntimeStrategy(run, selected.task.ID, selected.signature, prepared.runID, "reviewer", reviewResult.Strategy)
	run.render.Phase("reviewer", string(selected.reviewer), fmt.Sprintf("review complete [%.1fs]", reviewResult.DurationS))

	if reviewErr != nil {
		if isCancellationErr(reviewErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, reviewErr)
			return taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr, actor: reviewerActor, metrics: metrics}, nil //nolint:nilerr // error captured in outcome
		}
		metrics = append(metrics, domain.CostEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RunID:     prepared.runID,
			TaskID:    selected.task.ID,
			Agent:     string(selected.reviewer),
			Role:      "reviewer",
			DurationS: reviewResult.DurationS,
			Status:    "fail",
			CostUSD:   reviewResult.CostUSD,
		})
		return taskOutcome{
			kind:     taskOutcomeRetry,
			status:   "reviewer_crashed",
			message:  fmt.Sprintf("reviewer process failed: %v", reviewErr),
			feedback: fmt.Sprintf("reviewer process failed: %v", reviewErr),
			structuredFeedback: buildTaskFeedback(
				selected.task.ID,
				selected.retries+1,
				promptPhaseReview,
				*reviewerActor,
				"fail",
				fmt.Sprintf("reviewer process failed: %v", reviewErr),
				[]string{"Retry review after resolving the reviewer runtime failure."},
				"",
			),
			actor:        reviewerActor,
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Reviewer failed: %v (retry %d/%d)",
			renderArgs:   []any{reviewErr},
			metrics:      metrics,
		}, nil
	}

	metrics = append(metrics, domain.CostEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     prepared.runID,
		TaskID:    selected.task.ID,
		Agent:     string(selected.reviewer),
		Role:      "reviewer",
		DurationS: reviewResult.DurationS,
		Status:    "pass",
		CostUSD:   reviewResult.CostUSD,
	})
	if stalledOutcome, stalled := evaluateStallOutcome(run, selected, promptPhaseReview, reviewerOutput); stalled {
		stalledOutcome.metrics = append(stalledOutcome.metrics, metrics...)
		return stalledOutcome, nil
	}

	decision := domain.ParseReviewDecision(reviewerOutput)
	if !decision.Pass {
		status := "review_rejected"
		forceFailed := false
		parseClass := string(domain.ParseErrorRecoverable)
		if domain.IsReviewerDecisionParseFailure(decision) {
			status = "reviewer_parse_error"
			forceFailed = true
			parseClass = string(domain.ParseErrorNonRecoverable)
		}
		emitParseErrorClassEvent(run, selected.task.ID, promptPhaseReview, parseClass, decision.Reason)
		return taskOutcome{
			kind:     taskOutcomeRetry,
			status:   status,
			message:  decision.Reason,
			feedback: decision.Reason,
			structuredFeedback: buildTaskFeedback(
				selected.task.ID,
				selected.retries+1,
				promptPhaseReview,
				*reviewerActor,
				"fail",
				decision.Reason,
				decision.Hints,
				"",
			),
			actor:        reviewerActor,
			rollback:     true,
			forceFailed:  forceFailed,
			renderLevel:  "warn",
			renderFormat: "Review rejected: %s (retry %d/%d)",
			renderArgs:   []any{decision.Reason},
			metrics:      metrics,
		}, nil
	}

	return taskOutcome{
		kind:         taskOutcomeComplete,
		message:      fmt.Sprintf("task completed: %s", selected.task.Title),
		actor:        reviewerActor,
		renderFormat: "Completed: %s",
		renderArgs:   []any{selected.taskLabel},
		metrics:      metrics,
	}, nil
}

func isMergeConflictError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "merge conflict")
}

func requeueParallelConflict(run *activeRun, selected taskSelection, runID string, mergeErr error) error {
	if run == nil {
		return mergeErr
	}
	message := fmt.Sprintf("parallel merge conflict: %v", mergeErr)
	task := &run.state.Tasks[selected.index]
	task.Feedback = message
	nextStatus := domain.TaskPending
	task.Attempt++
	if task.Attempt >= run.options.MaxRetries {
		nextStatus = domain.TaskFailed
	}
	if err := run.transitions.TransitionTask(&run.state, selected.index, nextStatus); err != nil {
		return err
	}
	emitParallelMergeEvent(run, selected.task.ID, "conflict", message)
	if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    "parallel_conflict",
		TaskID:    selected.task.ID,
		Signature: selected.signature,
		RunID:     runID,
		Message:   message,
	}); err != nil {
		return err
	}
	if err := run.persistSnapshot("parallel_conflict", message); err != nil {
		return err
	}
	run.stats.TasksRejected++
	run.stats.Iterations++
	if nextStatus == domain.TaskFailed {
		run.render.Error(fmt.Sprintf("Parallel merge conflict exhausted retries: %s", selected.taskLabel))
	} else {
		run.render.Warn(fmt.Sprintf("Parallel merge conflict re-queued: %s", selected.taskLabel))
	}
	return nil
}

func emitParallelMergeEvent(run *activeRun, taskID, action, message string) {
	if run == nil || run.eventSink == nil {
		return
	}
	eventType := middleware.EventParallelMerge
	if strings.EqualFold(strings.TrimSpace(action), "conflict") {
		eventType = middleware.EventParallelConflict
	}
	run.eventSink.Emit(middleware.ExecutionEvent{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Type:          eventType,
		EventType:     string(eventType),
		RunID:         run.runID,
		TaskID:        strings.TrimSpace(taskID),
		Actor:         &domain.EventActor{Role: "system", Agent: "parallel-merge"},
		Action:        strings.TrimSpace(action),
		Message:       strings.TrimSpace(message),
		Data: map[string]any{
			"task_id": strings.TrimSpace(taskID),
			"action":  strings.TrimSpace(action),
		},
	})
}
