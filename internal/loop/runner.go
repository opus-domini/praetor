package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	}

	store := NewStore(normalized.StateRoot)
	lockPath, err := store.AcquireRunLock(planFile, normalized.Force)
	if err != nil {
		return RunnerStats{}, err
	}
	defer store.ReleaseRunLock(lockPath)

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

	stuck, err := store.DetectStuckTasks(state, normalized.MaxRetries)
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

	for {
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
				return stats, errors.New("plan is blocked: open tasks exist but none are runnable")
			}
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

		signature := TaskSignature(TaskKey(index, task))
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

		executorOutput, execErr := runtime.Run(ctx, AgentRequest{
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
		_ = writeText(filepath.Join(runDir, "executor.output.txt"), executorOutput)

		if execErr != nil {
			stats.TasksRejected++
			nextRetry, incErr := store.IncrementRetryCount(signature)
			if incErr != nil {
				return stats, incErr
			}
			_ = store.WriteFeedback(signature, fmt.Sprintf("executor process failed: %v", execErr))
			render.Error(fmt.Sprintf("Executor failed: %v (retry %d/%d)", execErr, nextRetry, normalized.MaxRetries))
			stats.Iterations++
			continue
		}

		result := ParseExecutorResult(executorOutput)
		if result == ExecutorResultFail {
			stats.TasksRejected++
			nextRetry, incErr := store.IncrementRetryCount(signature)
			if incErr != nil {
				return stats, incErr
			}
			_ = store.WriteFeedback(signature, "executor self-reported RESULT: FAIL")
			render.Error(fmt.Sprintf("Executor reported FAIL (retry %d/%d)", nextRetry, normalized.MaxRetries))
			stats.Iterations++
			continue
		}

		decision := ReviewDecision{Pass: true, Reason: "review skipped"}
		if reviewer != AgentNone {
			render.Phase("reviewer", string(reviewer), "reviewing task result")
			reviewerSystemPrompt := buildReviewerSystemPrompt()
			reviewerTaskPrompt := buildReviewerTaskPrompt(planFile, task, executorOutput, normalized.Workdir, plan.Title, progress)
			_ = writeText(filepath.Join(runDir, "reviewer.system.txt"), reviewerSystemPrompt)
			_ = writeText(filepath.Join(runDir, "reviewer.prompt.txt"), reviewerTaskPrompt)

			reviewerOutput, reviewErr := runtime.Run(ctx, AgentRequest{
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
			_ = writeText(filepath.Join(runDir, "reviewer.output.txt"), reviewerOutput)

			if reviewErr != nil {
				stats.TasksRejected++
				nextRetry, incErr := store.IncrementRetryCount(signature)
				if incErr != nil {
					return stats, incErr
				}
				_ = store.WriteFeedback(signature, fmt.Sprintf("reviewer process failed: %v", reviewErr))
				render.Error(fmt.Sprintf("Reviewer failed: %v (retry %d/%d)", reviewErr, nextRetry, normalized.MaxRetries))
				stats.Iterations++
				continue
			}

			decision = ParseReviewDecision(reviewerOutput)
		}

		if !decision.Pass {
			stats.TasksRejected++
			nextRetry, incErr := store.IncrementRetryCount(signature)
			if incErr != nil {
				return stats, incErr
			}
			_ = store.WriteFeedback(signature, decision.Reason)
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

		stats.TasksDone++
		stats.Iterations++
		render.Success(fmt.Sprintf("Completed: %s", taskLabel))
	}

	render.Summary(stats.TasksDone, stats.TasksRejected, stats.Iterations)
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
		normalized.MaxRetries = 3
	}
	if normalized.MaxIterations < 0 {
		return RunnerOptions{}, errors.New("max iterations cannot be negative")
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
