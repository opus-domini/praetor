package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Runner executes a dependency-aware plan with retries and review gates.
type Runner struct {
	runtime AgentRuntime
}

// NewRunner creates a loop runner.
func NewRunner(runtime AgentRuntime) *Runner {
	return &Runner{runtime: runtime}
}

type activeRun struct {
	planFile string
	plan     Plan
	options  RunnerOptions

	runtime     AgentRuntime
	render      *Renderer
	store       *Store
	transitions *TransitionRecorder
	isolation   *IsolationPolicy

	projectContext string
	state          State
	stats          RunnerStats
	totalCost      float64
	loopStart      time.Time
}

// Run executes a plan file until completion, blockage, or retry exhaustion.
func (r *Runner) Run(ctx context.Context, out io.Writer, planFile string, options RunnerOptions) (RunnerStats, error) {
	run, lock, cleanupRuntime, err := r.bootstrapRun(ctx, out, planFile, options)
	if err != nil {
		r.cleanupBootstrapFailure(out, run, lock, cleanupRuntime)
		return RunnerStats{}, err
	}
	defer cleanupRuntime()
	defer run.isolation.Cleanup()
	defer func() {
		if releaseErr := run.store.ReleaseRunLock(lock); releaseErr != nil {
			_, _ = fmt.Fprintf(out, "warning: failed to release lock: %v\n", releaseErr)
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

func (r *Runner) cleanupBootstrapFailure(out io.Writer, run activeRun, lock RunLock, cleanupRuntime func()) {
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
		_, _ = fmt.Fprintf(out, "warning: failed to release lock after bootstrap error: %v\n", err)
	}
}

func (r *Runner) bootstrapRun(ctx context.Context, out io.Writer, planFile string, options RunnerOptions) (activeRun, RunLock, func(), error) {
	planFile = strings.TrimSpace(planFile)
	if planFile == "" {
		return activeRun{}, RunLock{}, nil, errors.New("plan file is required")
	}

	cleanupRuntime := func() {}
	run := activeRun{planFile: planFile}

	plan, err := LoadPlan(planFile)
	if err != nil {
		return activeRun{}, RunLock{}, nil, err
	}
	run.plan = plan

	normalized, err := normalizeRunnerOptions(options)
	if err != nil {
		return run, RunLock{}, cleanupRuntime, err
	}
	render := NewRenderer(out, normalized.NoColor)
	run.options = normalized
	run.render = render

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, RunLock{}, cleanupRuntime, ctxErr
	}

	runtime := r.runtime
	if runtime == nil {
		runtime = newComposedRuntime(defaultAgents(), newTmuxRunner(normalized.TMUXSession))
	}
	run.runtime = runtime

	if sm, ok := runtime.(SessionManager); ok {
		if err := sm.EnsureSession(); err != nil {
			return run, RunLock{}, cleanupRuntime, err
		}
		cleanupRuntime = sm.Cleanup
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, RunLock{}, cleanupRuntime, ctxErr
	}

	if err := validateRequiredBinaries(normalized, plan); err != nil {
		return run, RunLock{}, cleanupRuntime, err
	}

	store := NewStore(normalized.StateRoot, normalized.CacheRoot)
	run.store = store

	lock, err := store.AcquireRunLock(planFile, normalized.Force)
	if err != nil {
		return run, RunLock{}, cleanupRuntime, err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, lock, cleanupRuntime, ctxErr
	}

	state, err := store.LoadOrInitializeState(planFile, plan)
	if err != nil {
		return run, lock, cleanupRuntime, err
	}
	run.state = state

	stats := RunnerStats{
		PlanFile:  planFile,
		StateFile: store.StateFile(planFile),
	}
	run.stats = stats

	stuck, err := store.DetectStuckTasks(planFile, state, normalized.MaxRetries)
	if err != nil {
		return run, lock, cleanupRuntime, err
	}
	if len(stuck) > 0 {
		return run, lock, cleanupRuntime, fmt.Errorf("tasks are stuck at retry limit:\n- %s", strings.Join(stuck, "\n- "))
	}

	isolation := NewIsolationPolicy(normalized.Workdir, normalized.Isolation)
	run.isolation = isolation
	if err := isolation.PruneOrphans(ctx); err != nil {
		return run, lock, cleanupRuntime, err
	}

	projectContext, truncated, projectContextErr := ReadProjectContext(normalized.Workdir)
	if projectContextErr != nil {
		render.Warn(fmt.Sprintf("Failed to read praetor.md: %v", projectContextErr))
	}
	if truncated {
		render.Warn("praetor.md exceeds 16 KiB; content truncated")
	}
	run.projectContext = projectContext

	render.Header("Praetor Loop")
	if planTitle := strings.TrimSpace(plan.Title); planTitle != "" {
		render.KV("Plan:", planTitle)
	}
	render.KV("Plan file:", planFile)
	render.KV("State:", stats.StateFile)
	render.KV("Progress:", fmt.Sprintf("%d/%d done", state.DoneCount(), len(state.Tasks)))
	if normalized.Isolation == IsolationWorktree {
		render.KV("Isolation:", "worktree")
	}
	if projectContext != "" {
		render.KV("Context:", "praetor.md")
	}
	if sm, ok := runtime.(SessionManager); ok {
		render.KV("tmux:", sm.SessionName())
	}

	run.transitions = NewTransitionRecorder(store, planFile)
	run.loopStart = time.Now()
	run.totalCost = 0
	return run, lock, cleanupRuntime, nil
}

func (r *Runner) runLoop(ctx context.Context, run *activeRun) error {
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if err := run.transitions.WriteCheckpoint(CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "canceled",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   ctxErr.Error(),
			}); err != nil {
				return fmt.Errorf("write cancellation checkpoint: %w", err)
			}
			return ctxErr
		}

		if run.options.MaxIterations > 0 && run.stats.Iterations >= run.options.MaxIterations {
			run.render.Warn(fmt.Sprintf("Stopped: max iterations reached (%d)", run.options.MaxIterations))
			break
		}

		stop, err := r.runIteration(ctx, run)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}

	run.stats.TotalCostUSD = run.totalCost
	run.stats.TotalDuration = time.Since(run.loopStart)
	run.render.Summary(run.stats.TasksDone, run.stats.TasksRejected, run.stats.Iterations, run.stats.TotalCostUSD, run.stats.TotalDuration)
	return nil
}

func (r *Runner) runIteration(ctx context.Context, run *activeRun) (bool, error) {
	index, task, ok := NextRunnableTask(run.state)
	if !ok {
		if run.state.OpenCount() == 0 {
			run.render.Success("All tasks completed")
			return true, nil
		}

		report := BlockedTasksReport(run.state, 5)
		if len(report) == 0 {
			if err := run.transitions.WriteCheckpoint(CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "blocked",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   "plan is blocked: open tasks exist but none are runnable",
			}); err != nil {
				return false, fmt.Errorf("write blocked checkpoint: %w", err)
			}
			return false, errors.New("plan is blocked: open tasks exist but none are runnable")
		}
		if err := run.transitions.WriteCheckpoint(CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "blocked",
			TaskID:    "",
			Signature: "",
			RunID:     "",
			Message:   fmt.Sprintf("plan is blocked by dependencies: %s", strings.Join(report, ", ")),
		}); err != nil {
			return false, fmt.Errorf("write blocked checkpoint: %w", err)
		}
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
	run.render.Task(progress, taskLabel, task.Title)

	// Transition to executing state before running the agent.
	if err := run.transitions.TransitionTask(&run.state, index, TaskExecuting, run.planFile); err != nil {
		return false, err
	}

	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d", selected.retries+1, run.options.MaxRetries))

	executorSystemPrompt := buildExecutorSystemPrompt(run.projectContext)
	executorTaskPrompt := buildExecutorTaskPrompt(run.planFile, selected.index, selected.task, selected.feedback, selected.retries, run.plan.Title, selected.progress, taskWorkdir)
	_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
	_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

	execResult, execErr := run.runtime.Run(ctx, AgentRequest{
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
			metrics: []CostEntry{
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

	result := ParseExecutorResult(executorOutput)
	if result != ExecutorResultPass {
		feedback := "executor self-reported RESULT: FAIL"
		if result == ExecutorResultUnknown {
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
			metrics: []CostEntry{
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

	if err := run.transitions.WriteMetric(CostEntry{
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

	decision := ReviewDecision{Pass: true, Reason: "review skipped"}
	if selected.reviewer != AgentNone {
		// Transition to reviewing state before running the reviewer.
		if err := run.transitions.TransitionTask(&run.state, selected.index, TaskReviewing, run.planFile); err != nil {
			return false, err
		}
		run.render.Phase("reviewer", string(selected.reviewer), "reviewing task result")
		gitDiff := CaptureGitDiff(taskWorkdir, 500)

		reviewerSystemPrompt := buildReviewerSystemPrompt(run.projectContext)
		reviewerTaskPrompt := buildReviewerTaskPrompt(run.planFile, selected.task, executorOutput, taskWorkdir, run.plan.Title, selected.progress, gitDiff)
		_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
		_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

		reviewResult, reviewErr := run.runtime.Run(ctx, AgentRequest{
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
				metrics: []CostEntry{
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

		if err := run.transitions.WriteMetric(CostEntry{
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

		decision = ParseReviewDecision(reviewerOutput)
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

func normalizeRunnerOptions(options RunnerOptions) (RunnerOptions, error) {
	normalized := options
	if strings.TrimSpace(normalized.Workdir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return RunnerOptions{}, fmt.Errorf("resolve working directory: %w", err)
		}
		normalized.Workdir = cwd
	}
	if strings.TrimSpace(normalized.StateRoot) == "" {
		stateRoot, err := ResolveStateRoot("", normalized.Workdir)
		if err != nil {
			return RunnerOptions{}, err
		}
		normalized.StateRoot = stateRoot
	}
	if strings.TrimSpace(normalized.CacheRoot) == "" {
		cacheRoot, err := ResolveCacheRoot("", normalized.Workdir)
		if err != nil {
			return RunnerOptions{}, err
		}
		normalized.CacheRoot = cacheRoot
	}
	if normalized.MaxRetries <= 0 {
		return RunnerOptions{}, errors.New("max retries must be greater than zero")
	}
	if normalized.MaxIterations < 0 {
		return RunnerOptions{}, errors.New("max iterations cannot be negative")
	}
	switch normalized.Isolation {
	case IsolationWorktree, IsolationOff:
	case "":
		normalized.Isolation = IsolationWorktree
	default:
		return RunnerOptions{}, fmt.Errorf("invalid isolation mode %q", normalized.Isolation)
	}

	normalized.DefaultExecutor = normalizeAgent(normalized.DefaultExecutor)
	if normalized.DefaultExecutor == "" {
		normalized.DefaultExecutor = AgentCodex
	}
	if _, ok := validExecutors[normalized.DefaultExecutor]; !ok {
		return RunnerOptions{}, fmt.Errorf("invalid default executor %q", normalized.DefaultExecutor)
	}

	normalized.DefaultReviewer = normalizeAgent(normalized.DefaultReviewer)
	if normalized.DefaultReviewer == "" {
		normalized.DefaultReviewer = AgentClaude
	}
	if _, ok := validReviewers[normalized.DefaultReviewer]; !ok {
		return RunnerOptions{}, fmt.Errorf("invalid default reviewer %q", normalized.DefaultReviewer)
	}

	if strings.TrimSpace(normalized.CodexBin) == "" {
		normalized.CodexBin = "codex"
	}
	if strings.TrimSpace(normalized.ClaudeBin) == "" {
		normalized.ClaudeBin = "claude"
	}
	normalized.TMUXSession = strings.TrimSpace(normalized.TMUXSession)
	if normalized.TMUXSession == "" {
		projectKey, err := ProjectRuntimeKeyForDir(normalized.Workdir)
		if err != nil {
			return RunnerOptions{}, err
		}
		normalized.TMUXSession = "praetor-" + projectKey
	}

	return normalized, nil
}

func validateRequiredBinaries(opts RunnerOptions, plan Plan) error {
	needed := map[string]string{}

	if opts.DefaultExecutor == AgentCodex {
		needed[opts.CodexBin] = "codex"
	}
	if opts.DefaultExecutor == AgentClaude {
		needed[opts.ClaudeBin] = "claude"
	}
	if !opts.SkipReview {
		if opts.DefaultReviewer == AgentCodex {
			needed[opts.CodexBin] = "codex"
		}
		if opts.DefaultReviewer == AgentClaude {
			needed[opts.ClaudeBin] = "claude"
		}
	}

	for _, task := range plan.Tasks {
		agent := normalizeAgent(task.Executor)
		if agent == AgentCodex {
			needed[opts.CodexBin] = "codex"
		}
		if agent == AgentClaude {
			needed[opts.ClaudeBin] = "claude"
		}
		if !opts.SkipReview {
			reviewer := normalizeAgent(task.Reviewer)
			if reviewer == AgentCodex {
				needed[opts.CodexBin] = "codex"
			}
			if reviewer == AgentClaude {
				needed[opts.ClaudeBin] = "claude"
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

func resolveExecutor(task StateTask, defaultExecutor Agent) (Agent, error) {
	executor := normalizeAgent(task.Executor)
	if executor == "" {
		executor = defaultExecutor
	}
	if executor == AgentNone {
		return "", errors.New("executor cannot be none")
	}
	if _, ok := validExecutors[executor]; !ok {
		return "", fmt.Errorf("invalid executor %q", executor)
	}
	return executor, nil
}

func resolveReviewer(task StateTask, defaultReviewer Agent, skipReview bool) (Agent, error) {
	if skipReview {
		return AgentNone, nil
	}

	reviewer := normalizeAgent(task.Reviewer)
	if reviewer == "" {
		reviewer = defaultReviewer
	}
	if _, ok := validReviewers[reviewer]; !ok {
		return "", fmt.Errorf("invalid reviewer %q", reviewer)
	}
	return reviewer, nil
}

func prepareRunDir(logRoot string, task StateTask, signature string) (string, error) {
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
