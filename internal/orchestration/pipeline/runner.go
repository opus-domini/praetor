package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/opus-domini/praetor/internal/workspace"
)

// Runner executes a dependency-aware plan with retries and review gates.
type Runner struct {
	runtime domain.AgentRuntime
}

// NewRunner creates a loop runner.
func NewRunner(runtime domain.AgentRuntime) *Runner {
	return &Runner{runtime: runtime}
}

type activeRun struct {
	planFile string
	plan     domain.Plan
	options  domain.RunnerOptions

	runtime     domain.AgentRuntime
	render      domain.RenderSink
	store       *localstate.Store
	transitions *TransitionRecorder
	isolation   *IsolationPolicy
	snapshot    *localstate.LocalSnapshotStore

	runID          string
	projectRoot    string
	manifestPath   string
	projectContext string
	state          domain.State
	stats          domain.RunnerStats
	totalCost      float64
	loopStart      time.Time
	stopRequested  bool
	stopReason     string
}

// Run executes a plan file until completion, blockage, or retry exhaustion.
func (r *Runner) Run(ctx context.Context, render domain.RenderSink, planFile string, options domain.RunnerOptions) (domain.RunnerStats, error) {
	run, lock, cleanupRuntime, err := r.bootstrapRun(ctx, render, planFile, options)
	if err != nil {
		r.cleanupBootstrapFailure(render, run, lock, cleanupRuntime)
		return domain.RunnerStats{}, err
	}
	defer cleanupRuntime()
	defer run.isolation.Cleanup()
	defer func() {
		if releaseErr := run.store.ReleaseRunLock(lock); releaseErr != nil {
			render.Warn(fmt.Sprintf("failed to release lock: %v", releaseErr))
		}
	}()

	if run.state.OpenCount() == 0 {
		run.render.Success(fmt.Sprintf("All tasks already completed: %s", run.planFile))
		return run.stats, nil
	}

	if err := r.runLoop(ctx, &run); err != nil {
		return run.stats, err
	}
	return run.stats, nil
}

func (r *Runner) cleanupBootstrapFailure(render domain.RenderSink, run activeRun, lock localstate.RunLock, cleanupRuntime func()) {
	if cleanupRuntime != nil {
		cleanupRuntime()
	}
	if run.isolation != nil {
		run.isolation.Cleanup()
	}
	if run.store == nil || strings.TrimSpace(lock.Path) == "" {
		return
	}
	if err := run.store.ReleaseRunLock(lock); err != nil {
		render.Warn(fmt.Sprintf("failed to release lock after bootstrap error: %v", err))
	}
}

func (r *Runner) bootstrapRun(ctx context.Context, render domain.RenderSink, planFile string, options domain.RunnerOptions) (activeRun, localstate.RunLock, func(), error) {
	planFile = strings.TrimSpace(planFile)
	if planFile == "" {
		return activeRun{}, localstate.RunLock{}, nil, errors.New("plan file is required")
	}

	cleanupRuntime := func() {}
	run := activeRun{planFile: planFile}

	normalized, err := normalizeRunnerOptions(options)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}
	run.options = normalized
	run.render = render

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, localstate.RunLock{}, cleanupRuntime, ctxErr
	}

	projectRoot, err := workspace.ResolveProjectRoot(normalized.Workdir)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}
	run.projectRoot = projectRoot
	run.runID = newRunID()

	manifest, manifestErr := workspace.ReadManifest(projectRoot)
	if manifestErr != nil {
		render.Warn(fmt.Sprintf("failed to read workspace manifest: %v", manifestErr))
	}
	if manifest.Truncated {
		render.Warn("workspace manifest exceeds 16 KiB; content truncated")
	}
	run.manifestPath = manifest.Path
	run.projectContext = manifest.Context

	runtime := r.runtime
	if runtime == nil {
		builtRuntime, buildErr := BuildAgentRuntime(normalized)
		if buildErr != nil {
			return run, localstate.RunLock{}, cleanupRuntime, buildErr
		}
		runtime = builtRuntime
	}
	run.runtime = runtime

	if sm, ok := runtime.(domain.SessionManager); ok {
		if err := sm.EnsureSession(); err != nil {
			return run, localstate.RunLock{}, cleanupRuntime, err
		}
		cleanupRuntime = sm.Cleanup
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, localstate.RunLock{}, cleanupRuntime, ctxErr
	}

	if strings.TrimSpace(normalized.Objective) != "" {
		planner, plannerErr := NewCognitiveAgent(normalized.PlannerAgent, runtime)
		if plannerErr != nil {
			return run, localstate.RunLock{}, cleanupRuntime, plannerErr
		}
		planned, planErr := planner.Plan(ctx, PlanRequest{
			Objective:      normalized.Objective,
			ProjectContext: run.projectContext,
			Workdir:        projectRoot,
			CodexBin:       normalized.CodexBin,
			ClaudeBin:      normalized.ClaudeBin,
		})
		if planErr != nil {
			return run, localstate.RunLock{}, cleanupRuntime, fmt.Errorf("planner failed: %w", planErr)
		}
		if err := writeGeneratedPlanFile(planFile, planned); err != nil {
			return run, localstate.RunLock{}, cleanupRuntime, err
		}
		render.KV("Objective:", normalized.Objective)
		render.KV("Planner:", string(normalized.PlannerAgent))
		render.Warn(fmt.Sprintf("Plan generated from objective and saved to %s", planFile))
	}

	plan, err := domain.LoadPlan(planFile)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}
	run.plan = plan

	if err := validateRequiredBinaries(normalized, plan); err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}

	store := localstate.NewStore(normalized.StateRoot, normalized.CacheRoot)
	run.store = store

	lock, err := store.AcquireRunLock(planFile, normalized.Force)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, lock, cleanupRuntime, ctxErr
	}

	state, err := store.LoadOrInitializeState(planFile, plan)
	if err != nil {
		return run, lock, cleanupRuntime, err
	}
	run.state = state

	if latest, path, snapshotErr := localstate.LoadLatestLocalSnapshot(projectRoot, planFile); snapshotErr != nil {
		render.Warn(fmt.Sprintf("failed to inspect local snapshots: %v", snapshotErr))
	} else if path != "" &&
		strings.TrimSpace(latest.PlanChecksum) == strings.TrimSpace(run.state.PlanChecksum) &&
		localstate.ParseTimestamp(latest.Timestamp).After(localstate.ParseTimestamp(run.state.UpdatedAt)) {
		run.state = latest.State
		if err := store.WriteState(planFile, run.state); err != nil {
			return run, lock, cleanupRuntime, fmt.Errorf("persist recovered state: %w", err)
		}
		render.Warn(fmt.Sprintf("Recovered state from local snapshot: %s", path))
	}

	stats := domain.RunnerStats{
		PlanFile:  planFile,
		StateFile: store.StateFile(planFile),
	}
	run.stats = stats

	snapshotStore := localstate.NewLocalSnapshotStore(projectRoot, run.runID)
	if err := snapshotStore.Init(planFile, run.state.PlanChecksum); err != nil {
		return run, lock, cleanupRuntime, err
	}
	if err := snapshotStore.WriteLock(lock.Token, os.Getpid()); err != nil {
		return run, lock, cleanupRuntime, err
	}
	run.snapshot = snapshotStore

	stuck, err := store.DetectStuckTasks(planFile, run.state, normalized.MaxRetries)
	if err != nil {
		return run, lock, cleanupRuntime, err
	}
	if len(stuck) > 0 {
		return run, lock, cleanupRuntime, fmt.Errorf("tasks are stuck at retry limit:\n- %s", strings.Join(stuck, "\n- "))
	}

	isolation := NewIsolationPolicy(projectRoot, normalized.Isolation)
	run.isolation = isolation
	if err := isolation.PruneOrphans(ctx); err != nil {
		return run, lock, cleanupRuntime, err
	}

	render.Header("Praetor Loop")
	if planTitle := strings.TrimSpace(plan.Title); planTitle != "" {
		render.KV("Plan:", planTitle)
	}
	render.KV("Plan file:", planFile)
	render.KV("State:", stats.StateFile)
	render.KV("Progress:", fmt.Sprintf("%d/%d done", run.state.DoneCount(), len(run.state.Tasks)))
	if normalized.Isolation == domain.IsolationWorktree {
		render.KV("Isolation:", "worktree")
	}
	render.KV("Runner:", string(normalized.RunnerMode))
	if manifest.Path != "" {
		render.KV("Context:", filepath.Base(manifest.Path))
	}
	if sm, ok := runtime.(domain.SessionManager); ok {
		render.KV("tmux:", sm.SessionName())
	}
	render.KV("Run:", run.runID)

	run.transitions = NewTransitionRecorder(store, planFile)
	run.loopStart = time.Now()
	run.totalCost = 0
	if err := run.persistSnapshot("bootstrap", "run initialized"); err != nil {
		return run, lock, cleanupRuntime, err
	}
	_ = run.appendSnapshotEvent("bootstrap", "", "run initialized")
	return run, lock, cleanupRuntime, nil
}

func (r *Runner) runLoop(ctx context.Context, run *activeRun) error {
	state := runnerStateCheckGuards
	for state != nil {
		next, err := state(ctx, r, run)
		if err != nil {
			return err
		}
		state = next
	}
	return nil
}

type runnerStateFn func(ctx context.Context, runner *Runner, run *activeRun) (runnerStateFn, error)

func runnerStateCheckGuards(ctx context.Context, runner *Runner, run *activeRun) (runnerStateFn, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		_ = run.appendSnapshotEvent("canceled", "", ctxErr.Error())
		_ = run.persistSnapshot("canceled", ctxErr.Error())
		if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "canceled",
			TaskID:    "",
			Signature: "",
			RunID:     "",
			Message:   ctxErr.Error(),
		}); err != nil {
			return nil, fmt.Errorf("write cancellation checkpoint: %w", err)
		}
		return nil, ctxErr
	}
	if err := run.persistSnapshot("loop", "iteration start"); err != nil {
		return nil, err
	}
	if run.options.MaxIterations > 0 && run.stats.Iterations >= run.options.MaxIterations {
		run.stopRequested = true
		run.stopReason = fmt.Sprintf("max iterations reached (%d)", run.options.MaxIterations)
		return runnerStateFinalize, nil
	}
	return runnerStateRunIteration, nil
}

func runnerStateRunIteration(ctx context.Context, runner *Runner, run *activeRun) (runnerStateFn, error) {
	stop, err := runner.runIteration(ctx, run)
	if err != nil {
		return nil, err
	}
	if stop {
		run.stopRequested = true
		if strings.TrimSpace(run.stopReason) == "" {
			run.stopReason = "run completed"
		}
		return runnerStateFinalize, nil
	}
	return runnerStateCheckGuards, nil
}

func runnerStateFinalize(_ context.Context, _ *Runner, run *activeRun) (runnerStateFn, error) {
	if strings.TrimSpace(run.stopReason) != "" && strings.Contains(run.stopReason, "max iterations reached") {
		_ = run.appendSnapshotEvent("stopped", "", run.stopReason)
		_ = run.persistSnapshot("stopped", run.stopReason)
		run.render.Warn(fmt.Sprintf("Stopped: %s", run.stopReason))
	}
	run.stats.TotalCostUSD = run.totalCost
	run.stats.TotalDuration = time.Since(run.loopStart)
	if err := run.persistSnapshot("finalize", "run finalized"); err != nil {
		return nil, err
	}
	_ = run.appendSnapshotEvent("finalized", "", "run finalized")
	run.render.Summary(run.stats.TasksDone, run.stats.TasksRejected, run.stats.Iterations, run.stats.TotalCostUSD, run.stats.TotalDuration)
	return nil, nil
}

func (r *Runner) runIteration(ctx context.Context, run *activeRun) (bool, error) {
	index, task, ok := domain.NextRunnableTask(run.state)
	if !ok {
		if run.state.OpenCount() == 0 {
			_ = run.appendSnapshotEvent("completed", "", "all tasks completed")
			_ = run.persistSnapshot("completed", "all tasks completed")
			run.render.Success("All tasks completed")
			return true, nil
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
				return false, fmt.Errorf("write blocked checkpoint: %w", err)
			}
			_ = run.appendSnapshotEvent("blocked", "", "plan is blocked: open tasks exist but none are runnable")
			_ = run.persistSnapshot("blocked", "plan is blocked: open tasks exist but none are runnable")
			return false, errors.New("plan is blocked: open tasks exist but none are runnable")
		}
		if err := run.transitions.WriteCheckpoint(domain.CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "blocked",
			TaskID:    "",
			Signature: "",
			RunID:     "",
			Message:   fmt.Sprintf("plan is blocked by dependencies: %s", strings.Join(report, ", ")),
		}); err != nil {
			return false, fmt.Errorf("write blocked checkpoint: %w", err)
		}
		_ = run.appendSnapshotEvent("blocked", "", "plan is blocked by dependencies")
		_ = run.persistSnapshot("blocked", "plan is blocked by dependencies")
		return false, fmt.Errorf("plan is blocked by dependencies:\n- %s", strings.Join(report, "\n- "))
	}

	executor, err := resolveExecutor(task, run.options.DefaultExecutor)
	if err != nil {
		return false, fmt.Errorf("task %s: %w", task.ID, err)
	}
	reviewer, err := resolveReviewer(task, run.options.DefaultReviewer, run.options.SkipReview)
	if err != nil {
		return false, fmt.Errorf("task %s: %w", task.ID, err)
	}

	signature := run.store.TaskSignatureForPlan(run.planFile, index, task)

	if task.Attempt >= run.options.MaxRetries {
		return false, fmt.Errorf("retry limit reached for task %s (%s)", task.ID, task.Title)
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
		return false, err
	}
	runID := filepath.Base(runDir)

	taskWorkdir, err := run.isolation.PrepareTask(ctx, runID, task.ID)
	if err != nil {
		return false, err
	}

	taskLabel := task.ID
	if strings.TrimSpace(taskLabel) == "" {
		taskLabel = fmt.Sprintf("#%d", index)
	}
	selected.taskLabel = taskLabel
	_ = run.appendSnapshotEvent("task_selected", task.ID, fmt.Sprintf("task selected: %s", task.Title))
	_ = run.persistSnapshot("task_selected", fmt.Sprintf("task selected: %s", task.ID))
	run.render.Task(progress, taskLabel, task.Title)

	// Transition to executing state before running the agent.
	if err := run.transitions.TransitionTask(&run.state, index, domain.TaskExecuting, run.planFile); err != nil {
		return false, err
	}

	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d", selected.retries+1, run.options.MaxRetries))

	executorSystemPrompt := BuildExecutorSystemPrompt(run.projectContext)
	executorTaskPrompt := BuildExecutorTaskPrompt(run.planFile, selected.index, selected.task, selected.feedback, selected.retries, run.plan.Title, selected.progress, taskWorkdir)
	_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
	_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

	execResult, execErr := run.runtime.Run(ctx, domain.AgentRequest{
		Role:         "execute",
		Agent:        selected.executor,
		Prompt:       executorTaskPrompt,
		SystemPrompt: executorSystemPrompt,
		Model:        selected.task.Model,
		Workdir:      taskWorkdir,
		RunDir:       runDir,
		OutputPrefix: "executor",
		TaskLabel:    taskLabel,
		CodexBin:     run.options.CodexBin,
		ClaudeBin:    run.options.ClaudeBin,
		Verbose:      run.options.Verbose,
	})
	executorOutput := execResult.Output
	run.totalCost += execResult.CostUSD
	_ = writeText(filepath.Join(runDir, "executor.output.txt"), executorOutput)

	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d [%.1fs]", selected.retries+1, run.options.MaxRetries, execResult.DurationS))

	if execErr != nil {
		if isCancellationErr(execErr) || ctx.Err() != nil {
			cancelErr := cancellationCause(ctx, execErr)
			_, applyErr := r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
				kind:      taskOutcomeCanceled,
				message:   cancelErr.Error(),
				cancelErr: cancelErr,
			})
			return false, applyErr
		}
		return r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "executor_crashed",
			message:      fmt.Sprintf("executor process failed: %v", execErr),
			feedback:     fmt.Sprintf("executor process failed: %v", execErr),
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Executor failed: %v (retry %d/%d)",
			renderArgs:   []any{execErr},
			metrics: []domain.CostEntry{
				{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					RunID:     runID,
					TaskID:    selected.task.ID,
					Agent:     string(selected.executor),
					Role:      "executor",
					DurationS: execResult.DurationS,
					Status:    "fail",
					CostUSD:   execResult.CostUSD,
				},
			},
		})
	}

	result := domain.ParseExecutorResult(executorOutput)
	if result != domain.ExecutorResultPass {
		feedback := "executor self-reported RESULT: FAIL"
		if result == domain.ExecutorResultUnknown {
			feedback = "executor output missing or invalid RESULT line"
		}
		return r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "executor_fail",
			message:      fmt.Sprintf("executor reported RESULT: %s", result),
			feedback:     feedback,
			rollback:     true,
			renderLevel:  "error",
			renderFormat: "Executor reported %s (retry %d/%d)",
			renderArgs:   []any{result},
			metrics: []domain.CostEntry{
				{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					RunID:     runID,
					TaskID:    selected.task.ID,
					Agent:     string(selected.executor),
					Role:      "executor",
					DurationS: execResult.DurationS,
					Status:    "fail",
					CostUSD:   execResult.CostUSD,
				},
			},
		})
	}

	if err := run.transitions.WriteMetric(domain.CostEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     runID,
		TaskID:    selected.task.ID,
		Agent:     string(selected.executor),
		Role:      "executor",
		DurationS: execResult.DurationS,
		Status:    "pass",
		CostUSD:   execResult.CostUSD,
	}); err != nil {
		return false, fmt.Errorf("write executor metric: %w", err)
	}

	if run.options.PostTaskHook != "" {
		run.render.Phase("hook", "post-task", run.options.PostTaskHook)
		hookPassed, hookFeedback := runPostTaskHook(ctx, run.options.PostTaskHook, taskWorkdir, runDir)
		if !hookPassed {
			if ctx.Err() != nil {
				cancelErr := cancellationCause(ctx, nil)
				_, applyErr := r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
					kind:      taskOutcomeCanceled,
					message:   cancelErr.Error(),
					cancelErr: cancelErr,
				})
				return false, applyErr
			}
			return r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
				kind:         taskOutcomeRetry,
				status:       "hook_failed",
				message:      "post-task hook failed",
				feedback:     hookFeedback,
				rollback:     true,
				renderLevel:  "error",
				renderFormat: "Post-task hook failed (retry %d/%d)",
			})
		}
	}

	decision := domain.ReviewDecision{Pass: true, Reason: "review skipped"}
	if selected.reviewer != domain.AgentNone {
		// Transition to reviewing state before running the reviewer.
		if err := run.transitions.TransitionTask(&run.state, selected.index, domain.TaskReviewing, run.planFile); err != nil {
			return false, err
		}
		run.render.Phase("reviewer", string(selected.reviewer), "reviewing task result")
		gitDiff := CaptureGitDiff(taskWorkdir, 500)

		reviewerSystemPrompt := BuildReviewerSystemPrompt(run.projectContext)
		reviewerTaskPrompt := BuildReviewerTaskPrompt(run.planFile, selected.task, executorOutput, taskWorkdir, run.plan.Title, selected.progress, gitDiff)
		_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
		_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

		reviewResult, reviewErr := run.runtime.Run(ctx, domain.AgentRequest{
			Role:         "review",
			Agent:        selected.reviewer,
			Prompt:       reviewerTaskPrompt,
			SystemPrompt: reviewerSystemPrompt,
			Model:        selected.task.Model,
			Workdir:      taskWorkdir,
			RunDir:       runDir,
			OutputPrefix: "reviewer",
			TaskLabel:    taskLabel,
			CodexBin:     run.options.CodexBin,
			ClaudeBin:    run.options.ClaudeBin,
			Verbose:      run.options.Verbose,
		})
		reviewerOutput := reviewResult.Output
		run.totalCost += reviewResult.CostUSD
		_ = writeText(filepath.Join(runDir, "reviewer.output.txt"), reviewerOutput)

		run.render.Phase("reviewer", string(selected.reviewer), fmt.Sprintf("review complete [%.1fs]", reviewResult.DurationS))

		if reviewErr != nil {
			if isCancellationErr(reviewErr) || ctx.Err() != nil {
				cancelErr := cancellationCause(ctx, reviewErr)
				_, applyErr := r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
					kind:      taskOutcomeCanceled,
					message:   cancelErr.Error(),
					cancelErr: cancelErr,
				})
				return false, applyErr
			}
			return r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
				kind:         taskOutcomeRetry,
				status:       "reviewer_crashed",
				message:      fmt.Sprintf("reviewer process failed: %v", reviewErr),
				feedback:     fmt.Sprintf("reviewer process failed: %v", reviewErr),
				rollback:     true,
				renderLevel:  "error",
				renderFormat: "Reviewer failed: %v (retry %d/%d)",
				renderArgs:   []any{reviewErr},
				metrics: []domain.CostEntry{
					{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						RunID:     runID,
						TaskID:    selected.task.ID,
						Agent:     string(selected.reviewer),
						Role:      "reviewer",
						DurationS: reviewResult.DurationS,
						Status:    "fail",
						CostUSD:   reviewResult.CostUSD,
					},
				},
			})
		}

		if err := run.transitions.WriteMetric(domain.CostEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RunID:     runID,
			TaskID:    selected.task.ID,
			Agent:     string(selected.reviewer),
			Role:      "reviewer",
			DurationS: reviewResult.DurationS,
			Status:    "pass",
			CostUSD:   reviewResult.CostUSD,
		}); err != nil {
			return false, fmt.Errorf("write reviewer metric: %w", err)
		}

		decision = domain.ParseReviewDecision(reviewerOutput)
	}

	if !decision.Pass {
		return r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
			kind:         taskOutcomeRetry,
			status:       "review_rejected",
			message:      decision.Reason,
			feedback:     decision.Reason,
			rollback:     true,
			renderLevel:  "warn",
			renderFormat: "Review rejected: %s (retry %d/%d)",
			renderArgs:   []any{decision.Reason},
		})
	}

	return r.applyTaskOutcome(ctx, run, selected, runID, taskOutcome{
		kind:         taskOutcomeComplete,
		message:      fmt.Sprintf("task completed: %s", selected.task.Title),
		renderFormat: "Completed: %s",
		renderArgs:   []any{selected.taskLabel},
	})
}

func normalizeRunnerOptions(options domain.RunnerOptions) (domain.RunnerOptions, error) {
	normalized := options
	if strings.TrimSpace(normalized.Workdir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return domain.RunnerOptions{}, fmt.Errorf("resolve working directory: %w", err)
		}
		normalized.Workdir = cwd
	}
	if strings.TrimSpace(normalized.StateRoot) == "" {
		stateRoot, err := localstate.ResolveStateRoot("", normalized.Workdir)
		if err != nil {
			return domain.RunnerOptions{}, err
		}
		normalized.StateRoot = stateRoot
	}
	if strings.TrimSpace(normalized.CacheRoot) == "" {
		cacheRoot, err := localstate.ResolveCacheRoot("", normalized.Workdir)
		if err != nil {
			return domain.RunnerOptions{}, err
		}
		normalized.CacheRoot = cacheRoot
	}
	if normalized.MaxRetries <= 0 {
		return domain.RunnerOptions{}, errors.New("max retries must be greater than zero")
	}
	if normalized.MaxIterations < 0 {
		return domain.RunnerOptions{}, errors.New("max iterations cannot be negative")
	}
	switch normalized.Isolation {
	case domain.IsolationWorktree, domain.IsolationOff:
	case "":
		normalized.Isolation = domain.IsolationWorktree
	default:
		return domain.RunnerOptions{}, fmt.Errorf("invalid isolation mode %q", normalized.Isolation)
	}
	switch normalized.RunnerMode {
	case domain.RunnerTMUX, domain.RunnerPTY, domain.RunnerDirect:
	case "":
		normalized.RunnerMode = domain.RunnerTMUX
	default:
		return domain.RunnerOptions{}, fmt.Errorf("invalid runner mode %q", normalized.RunnerMode)
	}

	normalized.DefaultExecutor = domain.NormalizeAgent(normalized.DefaultExecutor)
	if normalized.DefaultExecutor == "" {
		normalized.DefaultExecutor = domain.AgentCodex
	}
	if _, ok := domain.ValidExecutors[normalized.DefaultExecutor]; !ok {
		return domain.RunnerOptions{}, fmt.Errorf("invalid default executor %q", normalized.DefaultExecutor)
	}

	normalized.DefaultReviewer = domain.NormalizeAgent(normalized.DefaultReviewer)
	if normalized.DefaultReviewer == "" {
		normalized.DefaultReviewer = domain.AgentClaude
	}
	if _, ok := domain.ValidReviewers[normalized.DefaultReviewer]; !ok {
		return domain.RunnerOptions{}, fmt.Errorf("invalid default reviewer %q", normalized.DefaultReviewer)
	}

	normalized.PlannerAgent = domain.NormalizeAgent(normalized.PlannerAgent)
	if normalized.PlannerAgent == "" {
		normalized.PlannerAgent = domain.AgentClaude
	}
	if _, ok := domain.ValidExecutors[normalized.PlannerAgent]; !ok {
		return domain.RunnerOptions{}, fmt.Errorf("invalid planner agent %q", normalized.PlannerAgent)
	}
	normalized.Objective = strings.TrimSpace(normalized.Objective)

	if strings.TrimSpace(normalized.CodexBin) == "" {
		normalized.CodexBin = "codex"
	}
	if strings.TrimSpace(normalized.ClaudeBin) == "" {
		normalized.ClaudeBin = "claude"
	}
	if strings.TrimSpace(normalized.GeminiBin) == "" {
		normalized.GeminiBin = "gemini"
	}
	if strings.TrimSpace(normalized.OllamaURL) == "" {
		normalized.OllamaURL = "http://127.0.0.1:11434"
	}
	if strings.TrimSpace(normalized.OllamaModel) == "" {
		normalized.OllamaModel = "llama3"
	}
	if normalized.RunnerMode == domain.RunnerTMUX {
		if !isTMUXCompatibleAgent(normalized.DefaultExecutor) {
			return domain.RunnerOptions{}, fmt.Errorf("runner mode %q only supports codex/claude executors (got %q)", normalized.RunnerMode, normalized.DefaultExecutor)
		}
		if !isTMUXCompatibleAgent(normalized.DefaultReviewer) {
			return domain.RunnerOptions{}, fmt.Errorf("runner mode %q only supports codex/claude reviewers (got %q)", normalized.RunnerMode, normalized.DefaultReviewer)
		}
		if strings.TrimSpace(normalized.Objective) != "" && !isTMUXCompatibleAgent(normalized.PlannerAgent) {
			return domain.RunnerOptions{}, fmt.Errorf("runner mode %q only supports codex/claude planners (got %q)", normalized.RunnerMode, normalized.PlannerAgent)
		}
	}
	if normalized.RunnerMode == domain.RunnerTMUX {
		normalized.TMUXSession = strings.TrimSpace(normalized.TMUXSession)
		if normalized.TMUXSession == "" {
			projectKey, err := localstate.ProjectRuntimeKeyForDir(normalized.Workdir)
			if err != nil {
				return domain.RunnerOptions{}, err
			}
			normalized.TMUXSession = "praetor-" + projectKey
		}
	}

	return normalized, nil
}

func validateRequiredBinaries(opts domain.RunnerOptions, plan domain.Plan) error {
	needed := map[string]string{}
	if opts.RunnerMode == domain.RunnerTMUX {
		for idx, task := range plan.Tasks {
			executor := domain.NormalizeAgent(task.Executor)
			if executor != "" && !isTMUXCompatibleAgent(executor) {
				return fmt.Errorf("runner mode %q does not support tasks[%d].executor=%q; use --runner direct or --runner pty", opts.RunnerMode, idx, executor)
			}
			if !opts.SkipReview {
				reviewer := domain.NormalizeAgent(task.Reviewer)
				if reviewer != "" && !isTMUXCompatibleAgent(reviewer) {
					return fmt.Errorf("runner mode %q does not support tasks[%d].reviewer=%q; use --runner direct or --runner pty", opts.RunnerMode, idx, reviewer)
				}
			}
		}
	}

	if strings.TrimSpace(opts.Objective) != "" {
		switch domain.NormalizeAgent(opts.PlannerAgent) {
		case domain.AgentCodex:
			needed[opts.CodexBin] = "codex(planner)"
		case domain.AgentClaude:
			needed[opts.ClaudeBin] = "claude(planner)"
		case domain.AgentGemini:
			needed[opts.GeminiBin] = "gemini(planner)"
		}
	}

	if opts.DefaultExecutor == domain.AgentCodex {
		needed[opts.CodexBin] = "codex"
	}
	if opts.DefaultExecutor == domain.AgentClaude {
		needed[opts.ClaudeBin] = "claude"
	}
	if opts.DefaultExecutor == domain.AgentGemini {
		needed[opts.GeminiBin] = "gemini"
	}
	if !opts.SkipReview {
		if opts.DefaultReviewer == domain.AgentCodex {
			needed[opts.CodexBin] = "codex"
		}
		if opts.DefaultReviewer == domain.AgentClaude {
			needed[opts.ClaudeBin] = "claude"
		}
		if opts.DefaultReviewer == domain.AgentGemini {
			needed[opts.GeminiBin] = "gemini"
		}
	}

	for _, task := range plan.Tasks {
		agent := domain.NormalizeAgent(task.Executor)
		if agent == domain.AgentCodex {
			needed[opts.CodexBin] = "codex"
		}
		if agent == domain.AgentClaude {
			needed[opts.ClaudeBin] = "claude"
		}
		if agent == domain.AgentGemini {
			needed[opts.GeminiBin] = "gemini"
		}
		if !opts.SkipReview {
			reviewer := domain.NormalizeAgent(task.Reviewer)
			if reviewer == domain.AgentCodex {
				needed[opts.CodexBin] = "codex"
			}
			if reviewer == domain.AgentClaude {
				needed[opts.ClaudeBin] = "claude"
			}
			if reviewer == domain.AgentGemini {
				needed[opts.GeminiBin] = "gemini"
			}
		}
	}

	var missing []string
	for bin, label := range needed {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, fmt.Sprintf("%s (%s)", label, bin))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("required binaries not found: %s", strings.Join(missing, ", "))
	}
	return nil
}

func runPostTaskHook(ctx context.Context, hookPath, workdir, runDir string) (bool, string) {
	cmd := exec.CommandContext(ctx, hookPath)
	cmd.Dir = workdir

	stdoutFile := filepath.Join(runDir, "post-hook.stdout")
	stderrFile := filepath.Join(runDir, "post-hook.stderr")

	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()
	_ = os.WriteFile(stdoutFile, []byte(stdout), 0o644)
	_ = os.WriteFile(stderrFile, []byte(stderr), 0o644)

	if err == nil {
		return true, ""
	}

	output := strings.TrimSpace(stdout)
	lines := strings.Split(output, "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}
	feedback := "post-task hook failed"
	if len(lines) > 0 && strings.TrimSpace(strings.Join(lines, "")) != "" {
		feedback = strings.Join(lines, "\n")
	} else if trimmedErr := strings.TrimSpace(stderr); trimmedErr != "" {
		feedback = trimmedErr
	}
	return false, feedback
}

func resolveExecutor(task domain.StateTask, defaultExecutor domain.Agent) (domain.Agent, error) {
	executor := domain.NormalizeAgent(task.Executor)
	if executor == "" {
		executor = defaultExecutor
	}
	if executor == domain.AgentNone {
		return "", errors.New("executor cannot be none")
	}
	if _, ok := domain.ValidExecutors[executor]; !ok {
		return "", fmt.Errorf("invalid executor %q", executor)
	}
	return executor, nil
}

func resolveReviewer(task domain.StateTask, defaultReviewer domain.Agent, skipReview bool) (domain.Agent, error) {
	if skipReview {
		return domain.AgentNone, nil
	}

	reviewer := domain.NormalizeAgent(task.Reviewer)
	if reviewer == "" {
		reviewer = defaultReviewer
	}
	if _, ok := domain.ValidReviewers[reviewer]; !ok {
		return "", fmt.Errorf("invalid reviewer %q", reviewer)
	}
	return reviewer, nil
}

func isTMUXCompatibleAgent(agent domain.Agent) bool {
	switch domain.NormalizeAgent(agent) {
	case "", domain.AgentNone, domain.AgentCodex, domain.AgentClaude:
		return true
	default:
		return false
	}
}

func prepareRunDir(logRoot string, task domain.StateTask, signature string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102-150405")
	taskLabel := task.ID
	if strings.TrimSpace(taskLabel) == "" {
		taskLabel = "task"
	}
	taskLabel = sanitizePathToken(taskLabel)
	sigPart := shortToken(signature, 8)
	runID := fmt.Sprintf("%s-%s-%s", timestamp, taskLabel, sigPart)
	runDir := filepath.Join(logRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("create run log directory: %w", err)
	}
	return runDir, nil
}

func sanitizePathToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "task"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-")
	return replacer.Replace(value)
}

func shortToken(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func writeText(path, text string) error {
	return os.WriteFile(path, []byte(text), 0o644)
}

func writeGeneratedPlanFile(path string, plan domain.Plan) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("plan file path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create plan directory: %w", err)
	}
	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode generated plan: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write generated plan: %w", err)
	}
	return nil
}

func isCancellationErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func cancellationCause(ctx context.Context, fallback error) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if fallback != nil {
		return fallback
	}
	return context.Canceled
}

func (run *activeRun) persistSnapshot(phase, message string) error {
	if run == nil || run.snapshot == nil {
		return nil
	}
	snapshot := localstate.LocalSnapshot{
		RunID:        run.runID,
		PlanFile:     run.planFile,
		PlanChecksum: run.state.PlanChecksum,
		ProjectRoot:  run.projectRoot,
		Phase:        strings.TrimSpace(phase),
		Message:      strings.TrimSpace(message),
		Iteration:    run.stats.Iterations,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		State:        run.state,
	}
	return run.snapshot.Save(snapshot)
}

func (run *activeRun) appendSnapshotEvent(status, taskID, message string) error {
	if run == nil || run.snapshot == nil {
		return nil
	}
	return run.snapshot.AppendEvent(localstate.SnapshotEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     run.runID,
		Status:    strings.TrimSpace(status),
		TaskID:    strings.TrimSpace(taskID),
		Message:   strings.TrimSpace(message),
	})
}

func newRunID() string {
	return fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102-150405.000000000"), os.Getpid())
}
