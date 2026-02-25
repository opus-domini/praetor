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
	gitSafety   *GitSafetyPolicy

	state     State
	stats     RunnerStats
	totalCost float64
	loopStart time.Time
}

// Run executes a plan file until completion, blockage, or retry exhaustion.
func (r *Runner) Run(ctx context.Context, out io.Writer, planFile string, options RunnerOptions) (RunnerStats, error) {
	run, lock, cleanupRuntime, err := r.bootstrapRun(ctx, out, planFile, options)
	if err != nil {
		return RunnerStats{}, err
	}
	defer cleanupRuntime()
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

func (r *Runner) bootstrapRun(_ context.Context, out io.Writer, planFile string, options RunnerOptions) (activeRun, RunLock, func(), error) {
	planFile = strings.TrimSpace(planFile)
	if planFile == "" {
		return activeRun{}, RunLock{}, nil, errors.New("plan file is required")
	}

	plan, err := LoadPlan(planFile)
	if err != nil {
		return activeRun{}, RunLock{}, nil, err
	}

	normalized, err := normalizeRunnerOptions(options)
	if err != nil {
		return activeRun{}, RunLock{}, nil, err
	}
	render := NewRenderer(out, normalized.NoColor)

	runtime := r.runtime
	cleanupRuntime := func() {}
	if runtime == nil {
		runtime = NewTMUXAgentRuntime(normalized.TMUXSession)
	}
	if tmuxRuntime, ok := runtime.(*TMUXAgentRuntime); ok {
		if err := tmuxRuntime.EnsureSession(); err != nil {
			return activeRun{}, RunLock{}, nil, err
		}
		cleanupRuntime = tmuxRuntime.Cleanup
	}

	if err := validateRequiredBinaries(normalized, plan); err != nil {
		return activeRun{}, RunLock{}, cleanupRuntime, err
	}

	store := NewStore(normalized.StateRoot)
	lock, err := store.AcquireRunLock(planFile, normalized.Force)
	if err != nil {
		return activeRun{}, RunLock{}, cleanupRuntime, err
	}

	state, err := store.LoadOrInitializeState(planFile, plan)
	if err != nil {
		return activeRun{}, lock, cleanupRuntime, err
	}

	stats := RunnerStats{
		PlanFile:  planFile,
		StateFile: store.StateFile(planFile),
	}

	stuck, err := store.DetectStuckTasks(planFile, state, normalized.MaxRetries)
	if err != nil {
		return activeRun{}, lock, cleanupRuntime, err
	}
	if len(stuck) > 0 {
		return activeRun{}, lock, cleanupRuntime, fmt.Errorf("tasks are stuck at retry limit:\n- %s", strings.Join(stuck, "\n- "))
	}

	render.Header("Praetor Loop")
	if planTitle := strings.TrimSpace(plan.Title); planTitle != "" {
		render.KV("Plan:", planTitle)
	}
	render.KV("Plan file:", planFile)
	render.KV("State:", stats.StateFile)
	render.KV("Progress:", fmt.Sprintf("%d/%d done", state.DoneCount(), len(state.Tasks)))
	if tmuxRuntime, ok := runtime.(*TMUXAgentRuntime); ok {
		render.KV("tmux:", tmuxRuntime.SessionName())
	}

	gitSafetyEnabled := normalized.GitSafetyMode != GitSafetyModeOff
	if gitSafetyEnabled && normalized.GitSafetyMode == GitSafetyModeStrict {
		dirty, dirtyErr := store.GitWorktreeDirty(normalized.Workdir)
		if dirtyErr != nil {
			return activeRun{}, lock, cleanupRuntime, dirtyErr
		}
		if dirty {
			if !normalized.AllowDirty {
				return activeRun{}, lock, cleanupRuntime, errors.New("git safety strict mode requires a clean worktree; commit/stash changes or pass --allow-dirty-worktree")
			}
			gitSafetyEnabled = false
			render.Warn("Dirty worktree detected; git safety rollback disabled for this run")
		}
	}

	return activeRun{
		planFile:    planFile,
		plan:        plan,
		options:     normalized,
		runtime:     runtime,
		render:      render,
		store:       store,
		transitions: NewTransitionRecorder(store, planFile),
		gitSafety:   NewGitSafetyPolicy(store, normalized.Workdir, gitSafetyEnabled),
		state:       state,
		stats:       stats,
		loopStart:   time.Now(),
		totalCost:   0,
	}, lock, cleanupRuntime, nil
}

func (r *Runner) runLoop(ctx context.Context, run *activeRun) error {
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			run.transitions.WriteCheckpoint(CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "canceled",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   ctxErr.Error(),
			})
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
			run.transitions.WriteCheckpoint(CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "blocked",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   "plan is blocked: open tasks exist but none are runnable",
			})
			return false, errors.New("plan is blocked: open tasks exist but none are runnable")
		}
		run.transitions.WriteCheckpoint(CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "blocked",
			TaskID:    "",
			Signature: "",
			RunID:     "",
			Message:   fmt.Sprintf("plan is blocked by dependencies: %s", strings.Join(report, ", ")),
		})
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
	retries, err := run.store.ReadRetryCount(signature)
	if err != nil {
		return false, err
	}
	if retries >= run.options.MaxRetries {
		return false, fmt.Errorf("retry limit reached for task %s (%s)", task.ID, task.Title)
	}

	feedback, err := run.store.ReadFeedback(signature)
	if err != nil {
		return false, err
	}

	progress := fmt.Sprintf("%d/%d", run.state.DoneCount()+1, len(run.state.Tasks))
	selected := taskSelection{
		index:     index,
		task:      task,
		executor:  executor,
		reviewer:  reviewer,
		signature: signature,
		retries:   retries,
		feedback:  feedback,
		progress:  progress,
	}
	runDir, err := prepareRunDir(run.store.LogsDir(), task, signature)
	if err != nil {
		return false, err
	}
	runID := filepath.Base(runDir)

	if err := run.gitSafety.PrepareTask(runID); err != nil {
		return false, err
	}

	taskLabel := task.ID
	if strings.TrimSpace(taskLabel) == "" {
		taskLabel = fmt.Sprintf("#%d", index)
	}
	selected.taskLabel = taskLabel
	run.render.Task(progress, taskLabel, task.Title)
	run.render.Phase("executor", string(selected.executor), fmt.Sprintf("attempt %d/%d", selected.retries+1, run.options.MaxRetries))

	executorSystemPrompt := buildExecutorSystemPrompt()
	executorTaskPrompt := buildExecutorTaskPrompt(run.planFile, selected.index, selected.task, selected.feedback, selected.retries, run.plan.Title, selected.progress, run.options.Workdir)
	_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
	_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

	execResult, execErr := run.runtime.Run(ctx, AgentRequest{
		Role:         "execute",
		Agent:        selected.executor,
		Prompt:       executorTaskPrompt,
		SystemPrompt: executorSystemPrompt,
		Model:        selected.task.Model,
		Workdir:      run.options.Workdir,
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
			_, applyErr := r.applyTaskOutcome(run, selected, runID, taskOutcome{
				kind:      taskOutcomeCanceled,
				message:   cancelErr.Error(),
				cancelErr: cancelErr,
			})
			return false, applyErr
		}
		return r.applyTaskOutcome(run, selected, runID, taskOutcome{
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
		return r.applyTaskOutcome(run, selected, runID, taskOutcome{
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

	run.transitions.WriteMetric(CostEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RunID:     runID,
		TaskID:    selected.task.ID,
		Agent:     string(selected.executor),
		Role:      "executor",
		DurationS: execResult.DurationS,
		Status:    "pass",
		CostUSD:   execResult.CostUSD,
	})

	if run.options.PostTaskHook != "" {
		run.render.Phase("hook", "post-task", run.options.PostTaskHook)
		hookPassed, hookFeedback := runPostTaskHook(ctx, run.options.PostTaskHook, run.options.Workdir, runDir)
		if !hookPassed {
			if ctx.Err() != nil {
				cancelErr := cancellationCause(ctx, nil)
				_, applyErr := r.applyTaskOutcome(run, selected, runID, taskOutcome{
					kind:      taskOutcomeCanceled,
					message:   cancelErr.Error(),
					cancelErr: cancelErr,
				})
				return false, applyErr
			}
			return r.applyTaskOutcome(run, selected, runID, taskOutcome{
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
		run.render.Phase("reviewer", string(selected.reviewer), "reviewing task result")
		gitDiff := CaptureGitDiff(run.options.Workdir, 500)

		reviewerSystemPrompt := buildReviewerSystemPrompt()
		reviewerTaskPrompt := buildReviewerTaskPrompt(run.planFile, selected.task, executorOutput, run.options.Workdir, run.plan.Title, selected.progress, gitDiff)
		_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
		_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

		reviewResult, reviewErr := run.runtime.Run(ctx, AgentRequest{
			Role:         "review",
			Agent:        selected.reviewer,
			Prompt:       reviewerTaskPrompt,
			SystemPrompt: reviewerSystemPrompt,
			Model:        selected.task.Model,
			Workdir:      run.options.Workdir,
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
				_, applyErr := r.applyTaskOutcome(run, selected, runID, taskOutcome{
					kind:      taskOutcomeCanceled,
					message:   cancelErr.Error(),
					cancelErr: cancelErr,
				})
				return false, applyErr
			}
			return r.applyTaskOutcome(run, selected, runID, taskOutcome{
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

		run.transitions.WriteMetric(CostEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RunID:     runID,
			TaskID:    selected.task.ID,
			Agent:     string(selected.reviewer),
			Role:      "reviewer",
			DurationS: reviewResult.DurationS,
			Status:    "pass",
			CostUSD:   reviewResult.CostUSD,
		})

		decision = ParseReviewDecision(reviewerOutput)
	}

	if !decision.Pass {
		return r.applyTaskOutcome(run, selected, runID, taskOutcome{
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

	return r.applyTaskOutcome(run, selected, runID, taskOutcome{
		kind:            taskOutcomeComplete,
		message:         fmt.Sprintf("task completed: %s", selected.task.Title),
		renderFormat:    "Completed: %s",
		renderArgs:      []any{selected.taskLabel},
		discardSnapshot: true,
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
	if normalized.MaxRetries <= 0 {
		return RunnerOptions{}, errors.New("max retries must be greater than zero")
	}
	if normalized.MaxIterations < 0 {
		return RunnerOptions{}, errors.New("max iterations cannot be negative")
	}
	if !normalized.GitSafety {
		normalized.GitSafetyMode = GitSafetyModeOff
	}
	if normalized.GitSafetyMode == "" && normalized.GitSafety {
		normalized.GitSafetyMode = GitSafetyModeStrict
	}
	switch normalized.GitSafetyMode {
	case GitSafetyModeOff, GitSafetyModeStrict:
	default:
		return RunnerOptions{}, fmt.Errorf("invalid git safety mode %q", normalized.GitSafetyMode)
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
	runID := fmt.Sprintf("%s-%s-%s", timestamp, taskLabel, signature[:8])
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
