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

// Run executes a plan file until completion, blockage, or retry exhaustion.
func (r *Runner) Run(ctx context.Context, out io.Writer, planFile string, options RunnerOptions) (RunnerStats, error) {
	planFile = strings.TrimSpace(planFile)
	if planFile == "" {
		return RunnerStats{}, errors.New("plan file is required")
	}

	plan, err := LoadPlan(planFile)
	if err != nil {
		return RunnerStats{}, err
	}

	normalized, err := normalizeRunnerOptions(options)
	if err != nil {
		return RunnerStats{}, err
	}
	render := NewRenderer(out, normalized.NoColor)

	runtime := r.runtime
	if runtime == nil {
		runtime = NewTMUXAgentRuntime(normalized.TMUXSession)
	}
	if tmuxRuntime, ok := runtime.(*TMUXAgentRuntime); ok {
		if err := tmuxRuntime.EnsureSession(); err != nil {
			return RunnerStats{}, err
		}
		defer tmuxRuntime.Cleanup()
	}

	// Pre-flight binary validation (Step 15).
	if err := validateRequiredBinaries(normalized, plan); err != nil {
		return RunnerStats{}, err
	}

	store := NewStore(normalized.StateRoot)
	lock, err := store.AcquireRunLock(planFile, normalized.Force)
	if err != nil {
		return RunnerStats{}, err
	}
	defer func() {
		if releaseErr := store.ReleaseRunLock(lock); releaseErr != nil {
			_, _ = fmt.Fprintf(out, "warning: failed to release lock: %v\n", releaseErr)
		}
	}()

	state, err := store.LoadOrInitializeState(planFile, plan)
	if err != nil {
		return RunnerStats{}, err
	}

	stats := RunnerStats{
		PlanFile:  planFile,
		StateFile: store.StateFile(planFile),
	}

	if state.OpenCount() == 0 {
		render.Success(fmt.Sprintf("All tasks already completed: %s", planFile))
		return stats, nil
	}

	stuck, err := store.DetectStuckTasks(planFile, state, normalized.MaxRetries)
	if err != nil {
		return stats, err
	}
	if len(stuck) > 0 {
		return stats, fmt.Errorf("tasks are stuck at retry limit:\n- %s", strings.Join(stuck, "\n- "))
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
			return stats, dirtyErr
		}
		if dirty {
			if !normalized.AllowDirty {
				return stats, errors.New("git safety strict mode requires a clean worktree; commit/stash changes or pass --allow-dirty-worktree")
			}
			gitSafetyEnabled = false
			render.Warn("Dirty worktree detected; git safety rollback disabled for this run")
		}
	}

	var totalCost float64
	loopStart := time.Now()

	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			_ = store.WriteCheckpoint(planFile, CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "canceled",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   ctxErr.Error(),
			})
			return stats, ctxErr
		}

		if normalized.MaxIterations > 0 && stats.Iterations >= normalized.MaxIterations {
			render.Warn(fmt.Sprintf("Stopped: max iterations reached (%d)", normalized.MaxIterations))
			break
		}

		index, task, ok := NextRunnableTask(state)
		if !ok {
			if state.OpenCount() == 0 {
				render.Success("All tasks completed")
				break
			}

			report := BlockedTasksReport(state, 5)
			if len(report) == 0 {
				_ = store.WriteCheckpoint(planFile, CheckpointEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Status:    "blocked",
					TaskID:    "",
					Signature: "",
					RunID:     "",
					Message:   "plan is blocked: open tasks exist but none are runnable",
				})
				return stats, errors.New("plan is blocked: open tasks exist but none are runnable")
			}
			_ = store.WriteCheckpoint(planFile, CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "blocked",
				TaskID:    "",
				Signature: "",
				RunID:     "",
				Message:   fmt.Sprintf("plan is blocked by dependencies: %s", strings.Join(report, ", ")),
			})
			return stats, fmt.Errorf("plan is blocked by dependencies:\n- %s", strings.Join(report, "\n- "))
		}

		executor, err := resolveExecutor(task, normalized.DefaultExecutor)
		if err != nil {
			return stats, fmt.Errorf("task %s: %w", task.ID, err)
		}
		reviewer, err := resolveReviewer(task, normalized.DefaultReviewer, normalized.SkipReview)
		if err != nil {
			return stats, fmt.Errorf("task %s: %w", task.ID, err)
		}

		signature := store.TaskSignatureForPlan(planFile, index, task)
		retries, err := store.ReadRetryCount(signature)
		if err != nil {
			return stats, err
		}
		if retries >= normalized.MaxRetries {
			return stats, fmt.Errorf("retry limit reached for task %s (%s)", task.ID, task.Title)
		}

		feedback, err := store.ReadFeedback(signature)
		if err != nil {
			return stats, err
		}

		progress := fmt.Sprintf("%d/%d", state.DoneCount()+1, len(state.Tasks))
		runDir, err := prepareRunDir(store.LogsDir(), task, signature)
		if err != nil {
			return stats, err
		}
		runID := filepath.Base(runDir)

		// Git safety: save snapshot before executor (Step 9).
		if gitSafetyEnabled {
			if err := store.SaveGitSnapshot(runID, normalized.Workdir); err != nil {
				return stats, err
			}
		}

		taskLabel := task.ID
		if strings.TrimSpace(taskLabel) == "" {
			taskLabel = fmt.Sprintf("#%d", index)
		}
		render.Task(progress, taskLabel, task.Title)
		render.Phase("executor", string(executor), fmt.Sprintf("attempt %d/%d", retries+1, normalized.MaxRetries))

		executorSystemPrompt := buildExecutorSystemPrompt()
		executorTaskPrompt := buildExecutorTaskPrompt(planFile, index, task, feedback, retries, plan.Title, progress, normalized.Workdir)
		_ = writeText(filepath.Join(runDir, "executor.system.txt"), executorSystemPrompt)
		_ = writeText(filepath.Join(runDir, "executor.prompt.txt"), executorTaskPrompt)

		execResult, execErr := runtime.Run(ctx, AgentRequest{
			Role:         "execute",
			Agent:        executor,
			Prompt:       executorTaskPrompt,
			SystemPrompt: executorSystemPrompt,
			Model:        task.Model,
			Workdir:      normalized.Workdir,
			RunDir:       runDir,
			OutputPrefix: "executor",
			TaskLabel:    taskLabel,
			CodexBin:     normalized.CodexBin,
			ClaudeBin:    normalized.ClaudeBin,
			Verbose:      normalized.Verbose,
		})
		executorOutput := execResult.Output
		totalCost += execResult.CostUSD
		_ = writeText(filepath.Join(runDir, "executor.output.txt"), executorOutput)

		render.Phase("executor", string(executor), fmt.Sprintf("attempt %d/%d [%.1fs]", retries+1, normalized.MaxRetries, execResult.DurationS))

		if execErr != nil {
			if isCancellationErr(execErr) || ctx.Err() != nil {
				cancelErr := cancellationCause(ctx, execErr)
				_ = store.WriteCheckpoint(planFile, CheckpointEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Status:    "canceled",
					TaskID:    task.ID,
					Signature: signature,
					RunID:     runID,
					Message:   cancelErr.Error(),
				})
				return stats, cancelErr
			}
			stats.TasksRejected++
			nextRetry, incErr := store.IncrementRetryCount(signature)
			if incErr != nil {
				return stats, incErr
			}
			_ = store.WriteFeedback(signature, fmt.Sprintf("executor process failed: %v", execErr))
			if gitSafetyEnabled {
				if rbErr := store.RollbackGitSnapshot(runID, normalized.Workdir); rbErr != nil {
					render.Warn(fmt.Sprintf("git rollback failed: %v", rbErr))
				}
			}
			_ = store.WriteTaskMetrics(CostEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     runID,
				TaskID:    task.ID,
				Agent:     string(executor),
				Role:      "executor",
				DurationS: execResult.DurationS,
				Status:    "fail",
				CostUSD:   execResult.CostUSD,
			})
			_ = store.WriteCheckpoint(planFile, CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "executor_crashed",
				TaskID:    task.ID,
				Signature: signature,
				RunID:     runID,
				Message:   fmt.Sprintf("executor process failed: %v", execErr),
			})
			render.Error(fmt.Sprintf("Executor failed: %v (retry %d/%d)", execErr, nextRetry, normalized.MaxRetries))
			stats.Iterations++
			continue
		}

		result := ParseExecutorResult(executorOutput)
		if result != ExecutorResultPass {
			stats.TasksRejected++
			nextRetry, incErr := store.IncrementRetryCount(signature)
			if incErr != nil {
				return stats, incErr
			}
			if result == ExecutorResultUnknown {
				_ = store.WriteFeedback(signature, "executor output missing or invalid RESULT line")
			} else {
				_ = store.WriteFeedback(signature, "executor self-reported RESULT: FAIL")
			}
			if gitSafetyEnabled {
				if rbErr := store.RollbackGitSnapshot(runID, normalized.Workdir); rbErr != nil {
					render.Warn(fmt.Sprintf("git rollback failed: %v", rbErr))
				}
			}
			_ = store.WriteTaskMetrics(CostEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     runID,
				TaskID:    task.ID,
				Agent:     string(executor),
				Role:      "executor",
				DurationS: execResult.DurationS,
				Status:    "fail",
				CostUSD:   execResult.CostUSD,
			})
			_ = store.WriteCheckpoint(planFile, CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "executor_fail",
				TaskID:    task.ID,
				Signature: signature,
				RunID:     runID,
				Message:   fmt.Sprintf("executor reported RESULT: %s", result),
			})
			render.Error(fmt.Sprintf("Executor reported %s (retry %d/%d)", result, nextRetry, normalized.MaxRetries))
			stats.Iterations++
			continue
		}

		// Write executor pass metrics.
		_ = store.WriteTaskMetrics(CostEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RunID:     runID,
			TaskID:    task.ID,
			Agent:     string(executor),
			Role:      "executor",
			DurationS: execResult.DurationS,
			Status:    "pass",
			CostUSD:   execResult.CostUSD,
		})

		// Post-task hook (Step 12).
		if normalized.PostTaskHook != "" {
			render.Phase("hook", "post-task", normalized.PostTaskHook)
			hookPassed, hookFeedback := runPostTaskHook(ctx, normalized.PostTaskHook, normalized.Workdir, runDir)
			if !hookPassed {
				if ctx.Err() != nil {
					cancelErr := cancellationCause(ctx, nil)
					_ = store.WriteCheckpoint(planFile, CheckpointEntry{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Status:    "canceled",
						TaskID:    task.ID,
						Signature: signature,
						RunID:     runID,
						Message:   cancelErr.Error(),
					})
					return stats, cancelErr
				}
				stats.TasksRejected++
				nextRetry, incErr := store.IncrementRetryCount(signature)
				if incErr != nil {
					return stats, incErr
				}
				_ = store.WriteFeedback(signature, hookFeedback)
				if gitSafetyEnabled {
					if rbErr := store.RollbackGitSnapshot(runID, normalized.Workdir); rbErr != nil {
						render.Warn(fmt.Sprintf("git rollback failed: %v", rbErr))
					}
				}
				_ = store.WriteCheckpoint(planFile, CheckpointEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Status:    "hook_failed",
					TaskID:    task.ID,
					Signature: signature,
					RunID:     runID,
					Message:   "post-task hook failed",
				})
				render.Error(fmt.Sprintf("Post-task hook failed (retry %d/%d)", nextRetry, normalized.MaxRetries))
				stats.Iterations++
				continue
			}
		}

		decision := ReviewDecision{Pass: true, Reason: "review skipped"}
		if reviewer != AgentNone {
			render.Phase("reviewer", string(reviewer), "reviewing task result")

			// Capture git diff for reviewer prompt (Step 11).
			gitDiff := CaptureGitDiff(normalized.Workdir, 500)

			reviewerSystemPrompt := buildReviewerSystemPrompt()
			reviewerTaskPrompt := buildReviewerTaskPrompt(planFile, task, executorOutput, normalized.Workdir, plan.Title, progress, gitDiff)
			_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
			_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

			reviewResult, reviewErr := runtime.Run(ctx, AgentRequest{
				Role:         "review",
				Agent:        reviewer,
				Prompt:       reviewerTaskPrompt,
				SystemPrompt: reviewerSystemPrompt,
				Model:        task.Model,
				Workdir:      normalized.Workdir,
				RunDir:       runDir,
				OutputPrefix: "reviewer",
				TaskLabel:    taskLabel,
				CodexBin:     normalized.CodexBin,
				ClaudeBin:    normalized.ClaudeBin,
				Verbose:      normalized.Verbose,
			})
			reviewerOutput := reviewResult.Output
			totalCost += reviewResult.CostUSD
			_ = writeText(filepath.Join(runDir, "reviewer.output.txt"), reviewerOutput)

			render.Phase("reviewer", string(reviewer), fmt.Sprintf("review complete [%.1fs]", reviewResult.DurationS))

			if reviewErr != nil {
				if isCancellationErr(reviewErr) || ctx.Err() != nil {
					cancelErr := cancellationCause(ctx, reviewErr)
					_ = store.WriteCheckpoint(planFile, CheckpointEntry{
						Timestamp: time.Now().UTC().Format(time.RFC3339),
						Status:    "canceled",
						TaskID:    task.ID,
						Signature: signature,
						RunID:     runID,
						Message:   cancelErr.Error(),
					})
					return stats, cancelErr
				}
				stats.TasksRejected++
				nextRetry, incErr := store.IncrementRetryCount(signature)
				if incErr != nil {
					return stats, incErr
				}
				_ = store.WriteFeedback(signature, fmt.Sprintf("reviewer process failed: %v", reviewErr))
				if gitSafetyEnabled {
					if rbErr := store.RollbackGitSnapshot(runID, normalized.Workdir); rbErr != nil {
						render.Warn(fmt.Sprintf("git rollback failed: %v", rbErr))
					}
				}
				_ = store.WriteTaskMetrics(CostEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					RunID:     runID,
					TaskID:    task.ID,
					Agent:     string(reviewer),
					Role:      "reviewer",
					DurationS: reviewResult.DurationS,
					Status:    "fail",
					CostUSD:   reviewResult.CostUSD,
				})
				_ = store.WriteCheckpoint(planFile, CheckpointEntry{
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Status:    "reviewer_crashed",
					TaskID:    task.ID,
					Signature: signature,
					RunID:     runID,
					Message:   fmt.Sprintf("reviewer process failed: %v", reviewErr),
				})
				render.Error(fmt.Sprintf("Reviewer failed: %v (retry %d/%d)", reviewErr, nextRetry, normalized.MaxRetries))
				stats.Iterations++
				continue
			}

			// Write reviewer metrics.
			_ = store.WriteTaskMetrics(CostEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				RunID:     runID,
				TaskID:    task.ID,
				Agent:     string(reviewer),
				Role:      "reviewer",
				DurationS: reviewResult.DurationS,
				Status:    "pass",
				CostUSD:   reviewResult.CostUSD,
			})

			decision = ParseReviewDecision(reviewerOutput)
		}

		if !decision.Pass {
			stats.TasksRejected++
			nextRetry, incErr := store.IncrementRetryCount(signature)
			if incErr != nil {
				return stats, incErr
			}
			_ = store.WriteFeedback(signature, decision.Reason)
			if gitSafetyEnabled {
				if rbErr := store.RollbackGitSnapshot(runID, normalized.Workdir); rbErr != nil {
					render.Warn(fmt.Sprintf("git rollback failed: %v", rbErr))
				}
			}
			_ = store.WriteCheckpoint(planFile, CheckpointEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Status:    "review_rejected",
				TaskID:    task.ID,
				Signature: signature,
				RunID:     runID,
				Message:   decision.Reason,
			})
			render.Warn(fmt.Sprintf("Review rejected: %s (retry %d/%d)", decision.Reason, nextRetry, normalized.MaxRetries))
			stats.Iterations++
			continue
		}

		state.Tasks[index].Status = TaskStatusDone
		if err := store.WriteState(planFile, state); err != nil {
			return stats, err
		}
		if err := store.ClearRetryCount(signature); err != nil {
			return stats, err
		}
		if err := store.ClearFeedback(signature); err != nil {
			return stats, err
		}

		// Git safety: discard snapshot on success (Step 9).
		if gitSafetyEnabled {
			_ = store.DiscardGitSnapshot(runID)
		}

		_ = store.WriteCheckpoint(planFile, CheckpointEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Status:    "completed",
			TaskID:    task.ID,
			Signature: signature,
			RunID:     runID,
			Message:   fmt.Sprintf("task completed: %s", task.Title),
		})

		stats.TasksDone++
		stats.Iterations++
		render.Success(fmt.Sprintf("Completed: %s", taskLabel))
	}

	stats.TotalCostUSD = totalCost
	stats.TotalDuration = time.Since(loopStart)
	render.Summary(stats.TasksDone, stats.TasksRejected, stats.Iterations, stats.TotalCostUSD, stats.TotalDuration)
	return stats, nil
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
