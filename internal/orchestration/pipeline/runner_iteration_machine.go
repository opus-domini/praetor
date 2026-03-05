package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agent/middleware"
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

	// Per-task agent override (schema v2): if the plan task declares agents, use them.
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
		return nil, fmt.Errorf("task %s: %w", task.ID, err)
	}
	reviewer, err := resolveReviewer(reviewerAgent, run.options.SkipReview)
	if err != nil {
		return nil, fmt.Errorf("task %s: %w", task.ID, err)
	}

	signature := run.store.TaskSignatureForPlan(run.slug, index, task)
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

	if err := run.transitions.TransitionTask(&run.state, index, domain.TaskExecuting); err != nil {
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

	// Resolve per-task tool constraints from the plan task.
	var allowedTools, deniedTools []string
	if selected.index < len(run.plan.Tasks) {
		if c := run.plan.Tasks[selected.index].Constraints; c != nil {
			allowedTools = c.AllowedTools
			deniedTools = c.DeniedTools
		}
	}
	executorSystemPrompt := BuildExecutorSystemPrompt(run.promptEngine, run.projectContext, allowedTools, deniedTools)
	feedback := selected.feedback
	truncatedSections := make([]string, 0)
	if run.budgetManager != nil && strings.TrimSpace(feedback) != "" {
		promptWithoutFeedback := BuildExecutorTaskPrompt(run.promptEngine, run.slug, selected.index, selected.task, "", selected.retries, run.plan.Name, selected.progress, machine.taskWorkdir, run.plan.Quality.Required, run.plan.Quality.EvidenceFormat)
		adjustedFeedback, truncated := run.budgetManager.TruncateExecuteFeedback(promptWithoutFeedback, feedback)
		feedback = adjustedFeedback
		truncatedSections = append(truncatedSections, truncated...)
	}
	executorTaskPrompt := BuildExecutorTaskPrompt(run.promptEngine, run.slug, selected.index, selected.task, feedback, selected.retries, run.plan.Name, selected.progress, machine.taskWorkdir, run.plan.Quality.Required, run.plan.Quality.EvidenceFormat)
	if err := run.appendPerformanceEntry(promptPhaseExecute, executorTaskPrompt, truncatedSections); err != nil {
		run.render.Warn(fmt.Sprintf("failed to write performance metric: %v", err))
	}
	if run.options.Verbose && run.budgetManager != nil {
		run.render.Info(fmt.Sprintf("execute budget: %d/%d chars", len(executorTaskPrompt), run.budgetManager.Budget(promptPhaseExecute)))
	}
	_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
	_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

	// Per-task model override (schema v2).
	executorModel := run.options.ExecutorModel
	if selected.index < len(run.plan.Tasks) {
		if a := run.plan.Tasks[selected.index].Agents; a != nil && a.ExecutorModel != "" {
			executorModel = a.ExecutorModel
		}
	}
	execResult, execErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:             "execute",
		Agent:            selected.executor,
		Prompt:           executorTaskPrompt,
		SystemPrompt:     executorSystemPrompt,
		Model:            strings.TrimSpace(executorModel),
		Workdir:          machine.taskWorkdir,
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
	machine.executorOutput = execResult.Output
	run.totalCost += execResult.CostUSD
	_ = writeText(filepath.Join(runDir, "executor.output.txt"), machine.executorOutput)
	logRuntimeStrategy(run, selected.task.ID, selected.signature, machine.runID, "executor", execResult.Strategy)

	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d [%.1fs]", selected.retries+1, run.options.MaxRetries, execResult.DurationS))

	if execErr != nil {
		if isCancellationErr(execErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, execErr)
			machine.setOutcome(taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr})
			return iterationStateApplyOutcome, nil //nolint:nilerr // error captured in outcome
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
	if stalledOutcome, stalled := evaluateStallOutcome(run, selected, promptPhaseExecute, machine.executorOutput); stalled {
		machine.setOutcome(stalledOutcome)
		return iterationStateApplyOutcome, nil
	}

	result := domain.ParseExecutorResult(machine.executorOutput)
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
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       status,
			message:      fmt.Sprintf("executor reported RESULT: %s", result),
			feedback:     feedback,
			rollback:     true,
			forceFailed:  forceFailed,
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
				return iterationStateApplyOutcome, nil //nolint:nilerr // error captured in outcome
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

	if err := run.transitions.TransitionTask(&run.state, selected.index, domain.TaskReviewing); err != nil {
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
	gitDiff := CaptureGitDiff(machine.taskWorkdir, 0)
	executorOutputForPrompt := machine.executorOutput
	gitDiffForPrompt := gitDiff
	truncatedSections := make([]string, 0)
	if run.budgetManager != nil {
		overhead := BuildReviewerTaskPrompt(run.promptEngine, run.slug, selected.task, "", machine.taskWorkdir, run.plan.Name, selected.progress, "")
		execOut, diff, truncated := run.budgetManager.TruncateReviewSections(overhead, executorOutputForPrompt, gitDiffForPrompt)
		executorOutputForPrompt = execOut
		gitDiffForPrompt = diff
		truncatedSections = append(truncatedSections, truncated...)
	}

	// Check if "standards" is among the required quality gates.
	standardsGate := hasGate(run.plan.Quality.Required, "standards")
	reviewerSystemPrompt := BuildReviewerSystemPrompt(run.promptEngine, run.projectContext, standardsGate)
	reviewerTaskPrompt := BuildReviewerTaskPrompt(run.promptEngine, run.slug, selected.task, executorOutputForPrompt, machine.taskWorkdir, run.plan.Name, selected.progress, gitDiffForPrompt)
	if err := run.appendPerformanceEntry(promptPhaseReview, reviewerTaskPrompt, truncatedSections); err != nil {
		run.render.Warn(fmt.Sprintf("failed to write performance metric: %v", err))
	}
	if run.options.Verbose && run.budgetManager != nil {
		run.render.Info(fmt.Sprintf("review budget: %d/%d chars", len(reviewerTaskPrompt), run.budgetManager.Budget(promptPhaseReview)))
	}
	_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
	_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

	if gateReason, blocked := executeHostGates(ctx, run, selected.task.ID, machine.taskWorkdir, machine.executorOutput); blocked {
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "review_rejected",
			message:      gateReason,
			feedback:     gateReason,
			rollback:     true,
			renderLevel:  "warn",
			renderFormat: "Review rejected: %s (retry %d/%d)",
			renderArgs:   []any{gateReason},
		})
		return iterationStateApplyOutcome, nil
	}

	// Per-task reviewer model override (schema v2).
	reviewerModel := run.options.ReviewerModel
	if selected.index < len(run.plan.Tasks) {
		if a := run.plan.Tasks[selected.index].Agents; a != nil && a.ReviewerModel != "" {
			reviewerModel = a.ReviewerModel
		}
	}
	reviewResult, reviewErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:             "review",
		Agent:            selected.reviewer,
		Prompt:           reviewerTaskPrompt,
		SystemPrompt:     reviewerSystemPrompt,
		Model:            strings.TrimSpace(reviewerModel),
		Workdir:          machine.taskWorkdir,
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
	run.totalCost += reviewResult.CostUSD
	_ = writeText(filepath.Join(runDir, "reviewer.output.txt"), reviewerOutput)
	logRuntimeStrategy(run, selected.task.ID, selected.signature, machine.runID, "reviewer", reviewResult.Strategy)

	run.render.Phase("reviewer", string(selected.reviewer), fmt.Sprintf("review complete [%.1fs]", reviewResult.DurationS))

	if reviewErr != nil {
		if isCancellationErr(reviewErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, reviewErr)
			machine.setOutcome(taskOutcome{kind: taskOutcomeCanceled, message: cancelErr.Error(), cancelErr: cancelErr})
			return iterationStateApplyOutcome, nil //nolint:nilerr // error captured in outcome
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
	if stalledOutcome, stalled := evaluateStallOutcome(run, selected, promptPhaseReview, reviewerOutput); stalled {
		machine.setOutcome(stalledOutcome)
		return iterationStateApplyOutcome, nil
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
		machine.setOutcome(taskOutcome{
			kind:         taskOutcomeRetry,
			status:       status,
			message:      decision.Reason,
			feedback:     decision.Reason,
			rollback:     true,
			forceFailed:  forceFailed,
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

func evaluateStallOutcome(run *activeRun, selected taskSelection, phase, output string) (taskOutcome, bool) {
	if run == nil || run.stallDetector == nil || !run.options.StallDetection {
		return taskOutcome{}, false
	}
	stalled, similarity := run.stallDetector.Observe(selected.task.ID, phase, output)
	if !stalled {
		return taskOutcome{}, false
	}

	key := selected.task.ID + ":" + phase
	level := run.stallEscalations[key] + 1
	run.stallEscalations[key] = level
	action := "mark_failed"

	if level == 1 {
		if fallbackAgent, ok := stallFallbackAgent(run, phase); ok {
			action = "fallback"
			switch phase {
			case promptPhaseExecute:
				run.options.DefaultExecutor = fallbackAgent
			case promptPhaseReview:
				run.options.DefaultReviewer = fallbackAgent
			}
			emitTaskStalledEvent(run, selected.task.ID, phase, similarity, action)
			return taskOutcome{
				kind:         taskOutcomeRetry,
				status:       "stalled_retry",
				message:      fmt.Sprintf("task stalled (similarity %.2f): switched to fallback agent %s", similarity, fallbackAgent),
				feedback:     "Stall detected: output repeated across attempts. Regenerate with a different strategy.",
				rollback:     true,
				renderLevel:  "warn",
				renderFormat: "Task stalled (retry %d/%d)",
			}, true
		}
	}

	if level <= 2 && run.budgetManager != nil {
		action = "budget_reduced"
		switch phase {
		case promptPhaseExecute:
			run.budgetManager.executeChars = maxInt(run.budgetManager.executeChars/2, 1000)
		default:
			run.budgetManager.reviewChars = maxInt(run.budgetManager.reviewChars/2, 1000)
		}
		emitTaskStalledEvent(run, selected.task.ID, phase, similarity, action)
		return taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "stalled_retry",
			message:      fmt.Sprintf("task stalled (similarity %.2f): reduced context budget", similarity),
			feedback:     "Stall detected: output repeated across attempts. Focus only on unresolved issues.",
			rollback:     true,
			renderLevel:  "warn",
			renderFormat: "Task stalled (retry %d/%d)",
		}, true
	}

	emitTaskStalledEvent(run, selected.task.ID, phase, similarity, action)
	return taskOutcome{
		kind:         taskOutcomeRetry,
		status:       "stalled",
		message:      fmt.Sprintf("task marked failed due to stall (similarity %.2f)", similarity),
		feedback:     "Task failed: repeated outputs detected (stalled).",
		rollback:     true,
		forceFailed:  true,
		renderLevel:  "error",
		renderFormat: "Task stalled and failed (retry %d/%d)",
	}, true
}

func stallFallbackAgent(run *activeRun, phase string) (domain.Agent, bool) {
	if run == nil {
		return "", false
	}
	fallback := domain.NormalizeAgent(run.options.FallbackAgent)
	if fallback == "" || fallback == domain.AgentNone {
		return "", false
	}
	if phase == promptPhaseExecute {
		if _, ok := domain.ValidExecutors[fallback]; ok {
			return fallback, true
		}
		return "", false
	}
	if _, ok := domain.ValidReviewers[fallback]; ok {
		return fallback, true
	}
	return "", false
}

func emitTaskStalledEvent(run *activeRun, taskID, phase string, similarity float64, action string) {
	if run == nil || run.eventSink == nil {
		return
	}
	run.eventSink.Emit(middleware.ExecutionEvent{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Type:          middleware.EventTaskStalled,
		EventType:     string(middleware.EventTaskStalled),
		RunID:         run.runID,
		TaskID:        strings.TrimSpace(taskID),
		Phase:         strings.TrimSpace(phase),
		Similarity:    similarity,
		WindowSize:    run.options.StallWindow,
		Action:        strings.TrimSpace(action),
		Data: map[string]any{
			"task_id":     strings.TrimSpace(taskID),
			"phase":       strings.TrimSpace(phase),
			"similarity":  similarity,
			"window_size": run.options.StallWindow,
			"action":      strings.TrimSpace(action),
		},
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func hasGate(gates []string, name string) bool {
	for _, g := range gates {
		if strings.EqualFold(strings.TrimSpace(g), name) {
			return true
		}
	}
	return false
}

func executeHostGates(ctx context.Context, run *activeRun, taskID, workdir, executorOutput string) (string, bool) {
	if run == nil {
		return "", false
	}
	required := run.plan.Quality.Required
	optional := run.plan.Quality.Optional
	if len(required) == 0 && len(optional) == 0 {
		return "", false
	}

	commandMap := map[string]string{
		"tests":     strings.TrimSpace(run.options.GateTestsCmd),
		"lint":      strings.TrimSpace(run.options.GateLintCmd),
		"standards": strings.TrimSpace(run.options.GateStandardsCmd),
	}
	for gate, cmd := range run.plan.Quality.Commands {
		name := strings.ToLower(strings.TrimSpace(gate))
		if name == "" {
			continue
		}
		commandMap[name] = strings.TrimSpace(cmd)
	}

	timeout := run.options.Timeout
	gateRunner := NewHostGateRunner(commandMap)
	results := gateRunner.Run(ctx, strings.TrimSpace(workdir), required, optional, timeout)
	reported := domain.ParseGateEvidence(executorOutput)

	for _, result := range results {
		if strings.TrimSpace(result.Name) == "" {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(string(result.Status)))
		detail := strings.TrimSpace(result.Detail)

		// Backward compatibility: if host command is not configured, use executor-reported
		// evidence as auxiliary fallback for that gate.
		if result.Status == gateStatusMissing {
			if evidence, ok := reported[result.Name]; ok {
				status = strings.ToUpper(strings.TrimSpace(evidence.Status))
				if strings.TrimSpace(detail) == "" {
					detail = strings.TrimSpace(evidence.Detail)
				}
				if detail == "" {
					detail = "fallback to executor-reported evidence"
				}
			}
		}

		emitGateResultEvent(run, taskID, result.Name, status, detail, result.Required)
		if result.Required && status != "PASS" {
			reason := fmt.Sprintf("gate failed: %s", strings.TrimSpace(result.Name))
			if strings.TrimSpace(detail) != "" {
				reason = fmt.Sprintf("%s (%s)", reason, strings.TrimSpace(detail))
			}
			return reason, true
		}
	}

	return "", false
}

func emitParseErrorClassEvent(run *activeRun, taskID, phase, class, message string) {
	if run == nil || run.eventSink == nil {
		return
	}
	class = strings.TrimSpace(class)
	if class == "" {
		return
	}
	run.eventSink.Emit(middleware.ExecutionEvent{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Type:          middleware.EventAgentError,
		EventType:     string(middleware.EventAgentError),
		RunID:         run.runID,
		TaskID:        strings.TrimSpace(taskID),
		Phase:         strings.TrimSpace(phase),
		Error:         strings.TrimSpace(message),
		Data: map[string]any{
			"task_id":           strings.TrimSpace(taskID),
			"phase":             strings.TrimSpace(phase),
			"parse_error_class": class,
			"message":           strings.TrimSpace(message),
		},
	})
}

func emitGateResultEvent(run *activeRun, taskID, gateName, status, detail string, required bool) {
	if run == nil || run.eventSink == nil {
		return
	}
	run.eventSink.Emit(middleware.ExecutionEvent{
		SchemaVersion: 1,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Type:          middleware.EventGateResult,
		EventType:     string(middleware.EventGateResult),
		RunID:         run.runID,
		TaskID:        strings.TrimSpace(taskID),
		Phase:         promptPhaseReview,
		Action:        strings.TrimSpace(status),
		Message:       strings.TrimSpace(detail),
		Data: map[string]any{
			"task_id":  strings.TrimSpace(taskID),
			"gate":     strings.TrimSpace(gateName),
			"status":   strings.TrimSpace(status),
			"detail":   strings.TrimSpace(detail),
			"required": required,
		},
	})
}
