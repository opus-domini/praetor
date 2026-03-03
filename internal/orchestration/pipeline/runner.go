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

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/orchestration/fsm"
	"github.com/opus-domini/praetor/internal/prompt"
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
	slug    string
	plan    domain.Plan
	options domain.RunnerOptions

	runtime     domain.AgentRuntime
	render      domain.RenderSink
	store       *localstate.Store
	transitions *TransitionRecorder
	isolation   *IsolationPolicy
	snapshot    *localstate.LocalSnapshotStore

	runID             string
	projectRoot       string
	manifestPath      string
	manifestHash      string
	manifestTruncated bool
	projectContext    string
	state             domain.State
	stats             domain.RunnerStats
	totalCost         float64
	loopStart         time.Time
	stateTransitions  int
	stopRequested     bool
	stopReason        string
	eventSink         middleware.EventSink
	availableAgents   []agent.ID
	promptEngine      *prompt.Engine
	budgetManager     *ContextBudgetManager
	performancePath   string
	stallDetector     *StallDetector
	stallEscalations  map[string]int
}

// Run executes a plan slug until completion, blockage, or retry exhaustion.
func (r *Runner) Run(ctx context.Context, render domain.RenderSink, slug string, options domain.RunnerOptions) (domain.RunnerStats, error) {
	run, lock, cleanupRuntime, err := r.bootstrapRun(ctx, render, slug, options)
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

	if run.state.ActiveCount() == 0 {
		run.stats.Outcome = determineRunOutcome(run.state, nil)
		run.state.Outcome = run.stats.Outcome
		_ = run.store.WriteState(run.slug, run.state)
		run.render.Success(fmt.Sprintf("All tasks already completed: %s", run.slug))
		return run.stats, nil
	}

	if err := r.runLoop(ctx, &run); err != nil {
		run.stats.TotalCostUSD = run.totalCost
		run.stats.TotalDuration = time.Since(run.loopStart)
		run.stats.Outcome = determineRunOutcome(run.state, err)
		run.state.Outcome = run.stats.Outcome
		_ = run.store.WriteState(run.slug, run.state)
		_ = run.appendSnapshotEvent("failed", "", err.Error())
		_ = run.persistSnapshot("failed", err.Error())
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

func (r *Runner) bootstrapRun(ctx context.Context, render domain.RenderSink, slug string, options domain.RunnerOptions) (activeRun, localstate.RunLock, func(), error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return activeRun{}, localstate.RunLock{}, nil, errors.New("plan slug is required")
	}

	cleanupRuntime := func() {}
	run := activeRun{slug: slug}

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

	promptDir := filepath.Join(projectRoot, ".praetor", "prompts")
	run.promptEngine, _ = prompt.NewEngine(promptDir)

	store := localstate.NewStore(normalized.ProjectHome)
	run.store = store

	if pruneErr := localstate.PruneLocalSnapshots(store.RuntimeDir(), normalized.KeepLastRuns); pruneErr != nil {
		render.Warn(fmt.Sprintf("failed to prune local snapshots: %v", pruneErr))
	}

	manifest, manifestErr := workspace.ReadManifest(projectRoot)
	if manifestErr != nil {
		render.Warn(fmt.Sprintf("failed to read workspace manifest: %v", manifestErr))
	}
	if manifest.Truncated {
		render.Warn("workspace manifest exceeds 16 KiB; content truncated")
	}
	run.manifestPath = manifest.Path
	run.manifestHash = manifest.Hash
	run.manifestTruncated = manifest.Truncated
	run.projectContext = manifest.Context

	// Phase 5: Probe available agents for intelligent routing
	prober := agent.NewProber(agent.WithTimeout(3 * time.Second))
	probeResults := prober.ProbeAll(ctx, binaryOverridesFromOpts(normalized), restEndpointsFromOpts(normalized))
	var availableAgents []agent.ID
	for _, pr := range probeResults {
		if pr.Healthy() {
			availableAgents = append(availableAgents, pr.ID)
		}
	}
	run.availableAgents = availableAgents

	// Phase 4+: runtime diagnostics sink shared by middleware and runner lifecycle.
	var eventSink middleware.EventSink = middleware.NopSink{}
	runRoot := filepath.Join(store.RuntimeDir(), run.runID)
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, fmt.Errorf("create runtime directory: %w", err)
	}
	eventsPath := filepath.Join(runRoot, "events.jsonl")
	if jsonlSink, sinkErr := middleware.NewJSONLSink(eventsPath); sinkErr == nil {
		eventSink = jsonlSink
	} else {
		render.Warn(fmt.Sprintf("failed to create event sink: %v", sinkErr))
	}
	run.eventSink = eventSink

	runtime := r.runtime
	if runtime == nil {
		builtRuntime, buildErr := BuildAgentRuntimeWithDeps(normalized, RuntimeDeps{
			EventSink: eventSink,
		})
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
	if closer, ok := eventSink.(interface{ Close() error }); ok {
		previousCleanup := cleanupRuntime
		cleanupRuntime = func() {
			previousCleanup()
			_ = closer.Close()
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, localstate.RunLock{}, cleanupRuntime, ctxErr
	}

	planFile := store.PlanFile(slug)

	if strings.TrimSpace(normalized.Objective) != "" {
		planner, plannerErr := NewCognitiveAgent(normalized.PlannerAgent, runtime, WithPromptEngine(run.promptEngine))
		if plannerErr != nil {
			return run, localstate.RunLock{}, cleanupRuntime, plannerErr
		}
		planned, planErr := planner.Plan(ctx, PlanRequest{
			Objective:      normalized.Objective,
			ProjectContext: run.projectContext,
			Workdir:        projectRoot,
			Model:          normalized.PlannerModel,
			CodexBin:       normalized.CodexBin,
			ClaudeBin:      normalized.ClaudeBin,
		})
		if planErr != nil {
			return run, localstate.RunLock{}, cleanupRuntime, fmt.Errorf("planner failed: %w", planErr)
		}
		planned = enrichGeneratedPlan(planned, normalized)
		if err := domain.ValidatePlan(planned); err != nil {
			return run, localstate.RunLock{}, cleanupRuntime, fmt.Errorf("planner generated invalid plan: %w", err)
		}
		if err := writeGeneratedPlanFile(planFile, planned); err != nil {
			return run, localstate.RunLock{}, cleanupRuntime, err
		}
		render.KV("Objective:", normalized.Objective)
		render.KV("Planner:", string(normalized.PlannerAgent))
		render.Info(fmt.Sprintf("Plan generated from objective and saved to %s", planFile))
	}

	plan, err := domain.LoadPlan(planFile)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}
	run.plan = plan
	run.options, err = mergeRunnerOptionsWithPlan(run.options, plan)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}

	if err := validateRequiredBinaries(run.options, plan); err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}

	lock, err := store.AcquireRunLock(slug, normalized.Force)
	if err != nil {
		return run, localstate.RunLock{}, cleanupRuntime, err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return run, lock, cleanupRuntime, ctxErr
	}

	state, err := store.LoadOrInitializeState(slug, plan)
	if err != nil {
		return run, lock, cleanupRuntime, err
	}
	run.state = state

	if latest, path, snapshotErr := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), slug); snapshotErr != nil {
		render.Warn(fmt.Sprintf("failed to inspect local snapshots: %v", snapshotErr))
	} else if path != "" &&
		strings.TrimSpace(latest.PlanChecksum) == strings.TrimSpace(run.state.PlanChecksum) &&
		localstate.ParseTimestamp(latest.Timestamp).After(localstate.ParseTimestamp(run.state.UpdatedAt)) {
		run.state = latest.State
		if err := store.WriteState(slug, run.state); err != nil {
			return run, lock, cleanupRuntime, fmt.Errorf("persist recovered state: %w", err)
		}
		render.Info(fmt.Sprintf("Recovered state from local snapshot: %s", path))
	}
	if run.state.ActiveCount() > 0 {
		if run.state.Outcome != "" {
			run.state.Outcome = ""
			if err := store.WriteState(slug, run.state); err != nil {
				return run, lock, cleanupRuntime, err
			}
		}
	}

	stats := domain.RunnerStats{
		PlanSlug:  slug,
		StateFile: store.StateFile(slug),
	}
	run.stats = stats

	snapshotStore := localstate.NewLocalSnapshotStore(store.RuntimeDir(), run.runID)
	if err := snapshotStore.Init(slug, run.state.PlanChecksum); err != nil {
		return run, lock, cleanupRuntime, err
	}
	if err := snapshotStore.WriteLock(lock.Token, os.Getpid()); err != nil {
		return run, lock, cleanupRuntime, err
	}
	run.snapshot = snapshotStore
	run.budgetManager = NewContextBudgetManager(run.options.BudgetExecute, run.options.BudgetReview)
	diagnosticsDir := filepath.Join(snapshotStore.RootDir(), "diagnostics")
	if err := os.MkdirAll(diagnosticsDir, 0o755); err != nil {
		return run, lock, cleanupRuntime, err
	}
	run.performancePath = filepath.Join(diagnosticsDir, "performance.jsonl")
	if run.options.StallDetection {
		run.stallDetector = NewStallDetector(run.options.StallWindow, run.options.StallThreshold)
		run.stallEscalations = make(map[string]int)
	}

	stuck, err := store.DetectStuckTasks(slug, run.state, run.options.MaxRetries)
	if err != nil {
		return run, lock, cleanupRuntime, err
	}
	if len(stuck) > 0 {
		return run, lock, cleanupRuntime, fmt.Errorf("tasks are stuck at retry limit:\n- %s", strings.Join(stuck, "\n- "))
	}

	isolation := NewIsolationPolicy(projectRoot, store.WorktreesDir(), normalized.Isolation)
	run.isolation = isolation
	if err := isolation.PruneOrphans(ctx); err != nil {
		return run, lock, cleanupRuntime, err
	}

	render.Header("Praetor Loop")
	if planName := strings.TrimSpace(plan.Name); planName != "" {
		render.KV("Plan:", planName)
	}
	if planSummary := strings.TrimSpace(plan.Summary); planSummary != "" {
		render.KV("Summary:", planSummary)
	}
	render.KV("Executor:", formatAgentWithModel(run.options.DefaultExecutor, run.options.ExecutorModel))
	if run.options.SkipReview {
		render.KV("Reviewer:", "none (disabled)")
	} else {
		render.KV("Reviewer:", formatAgentWithModel(run.options.DefaultReviewer, run.options.ReviewerModel))
	}
	render.KV("Planner:", formatAgentWithModel(run.options.PlannerAgent, run.options.PlannerModel))
	render.KV("Retries:", fmt.Sprintf("%d per task", run.options.MaxRetries))
	render.KV("Budget:", fmt.Sprintf("execute=%d review=%d", run.options.BudgetExecute, run.options.BudgetReview))
	if run.options.StallDetection {
		render.KV("Stall:", fmt.Sprintf("on (window=%d threshold=%.2f)", run.options.StallWindow, run.options.StallThreshold))
	} else {
		render.KV("Stall:", "off")
	}
	render.KV("Slug:", slug)
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
		render.KV("Tmux:", sm.SessionName())
	}
	render.KV("Run:", run.runID)

	run.transitions = NewTransitionRecorder(store, slug)
	run.loopStart = time.Now()
	run.totalCost = 0
	if err := run.persistSnapshot("bootstrap", "run initialized"); err != nil {
		return run, lock, cleanupRuntime, err
	}
	_ = run.appendSnapshotEvent("bootstrap", "", "run initialized")
	return run, lock, cleanupRuntime, nil
}

func formatAgentWithModel(agent domain.Agent, model string) string {
	normalized := strings.TrimSpace(string(domain.NormalizeAgent(agent)))
	if normalized == "" {
		normalized = "-"
	}
	modelName := strings.TrimSpace(model)
	if modelName == "" {
		modelName = "default"
	}
	return fmt.Sprintf("%s (model=%s)", normalized, modelName)
}

func (r *Runner) runLoop(ctx context.Context, run *activeRun) error {
	machine := runnerLoopMachine{
		runner: r,
		run:    run,
		next:   runnerStateCheckGuards,
	}
	return fsm.Run(ctx, &machine, runnerLoopStep)
}

type runnerStateFn func(ctx context.Context, runner *Runner, run *activeRun) (runnerStateFn, error)

type runnerLoopMachine struct {
	runner *Runner
	run    *activeRun
	next   runnerStateFn
}

func runnerLoopStep(ctx context.Context, machine *runnerLoopMachine) (fsm.StateFn[runnerLoopMachine], error) {
	if machine == nil || machine.next == nil {
		return nil, nil
	}
	if machine.run.options.MaxTransitions > 0 && machine.run.stateTransitions >= machine.run.options.MaxTransitions {
		return nil, fmt.Errorf("max transitions reached (%d)", machine.run.options.MaxTransitions)
	}
	machine.run.stateTransitions++
	next, err := machine.next(ctx, machine.runner, machine.run)
	if err != nil {
		return nil, err
	}
	machine.next = next
	if machine.next == nil {
		return nil, nil
	}
	return runnerLoopStep, nil
}

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
	run.stats.Outcome = determineRunOutcome(run.state, nil)
	if strings.Contains(run.stopReason, "max iterations reached") {
		run.stats.Outcome = domain.RunFailed
	}
	run.state.Outcome = run.stats.Outcome
	if err := run.store.WriteState(run.slug, run.state); err != nil {
		return nil, err
	}
	if err := run.persistSnapshot("finalize", "run finalized"); err != nil {
		return nil, err
	}
	_ = run.appendSnapshotEvent("finalized", "", "run finalized")
	run.render.Summary(run.stats.TasksDone, run.stats.TasksRejected, run.stats.Iterations, run.stats.TotalCostUSD, run.stats.TotalDuration)
	return nil, nil
}

func determineRunOutcome(state domain.State, runErr error) domain.RunOutcome {
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return domain.RunCanceled
		}
		return domain.RunFailed
	}
	if state.ActiveCount() == 0 {
		if state.FailedCount() > 0 {
			return domain.RunPartial
		}
		return domain.RunSuccess
	}
	return domain.RunFailed
}

func (r *Runner) runIteration(ctx context.Context, run *activeRun) (bool, error) {
	machine := iterationMachine{
		runner: r,
		run:    run,
		next:   iterationStateSelectTask,
	}
	if err := fsm.Run(ctx, &machine, iterationStep); err != nil {
		return false, err
	}
	return machine.stop, nil
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
	if strings.TrimSpace(normalized.ProjectHome) == "" {
		projectHome, err := localstate.ResolveProjectHome("", normalized.Workdir)
		if err != nil {
			return domain.RunnerOptions{}, err
		}
		normalized.ProjectHome = projectHome
	}
	if normalized.MaxRetries <= 0 {
		return domain.RunnerOptions{}, errors.New("max retries must be greater than zero")
	}
	if normalized.MaxIterations < 0 {
		return domain.RunnerOptions{}, errors.New("max iterations cannot be negative")
	}
	if normalized.MaxTransitions < 0 {
		return domain.RunnerOptions{}, errors.New("max transitions cannot be negative")
	}
	if normalized.KeepLastRuns < 0 {
		return domain.RunnerOptions{}, errors.New("keep-last-runs cannot be negative")
	}
	if normalized.Timeout < 0 {
		return domain.RunnerOptions{}, errors.New("timeout cannot be negative")
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
	normalized.ExecutorModel = strings.TrimSpace(normalized.ExecutorModel)
	normalized.ReviewerModel = strings.TrimSpace(normalized.ReviewerModel)
	normalized.PlannerModel = strings.TrimSpace(normalized.PlannerModel)

	if strings.TrimSpace(normalized.CodexBin) == "" {
		normalized.CodexBin = "codex"
	}
	if strings.TrimSpace(normalized.ClaudeBin) == "" {
		normalized.ClaudeBin = "claude"
	}
	if strings.TrimSpace(normalized.CopilotBin) == "" {
		normalized.CopilotBin = "copilot"
	}
	if strings.TrimSpace(normalized.GeminiBin) == "" {
		normalized.GeminiBin = "gemini"
	}
	if strings.TrimSpace(normalized.KimiBin) == "" {
		normalized.KimiBin = "kimi"
	}
	if strings.TrimSpace(normalized.OpenCodeBin) == "" {
		normalized.OpenCodeBin = "opencode"
	}
	if strings.TrimSpace(normalized.OpenRouterURL) == "" {
		normalized.OpenRouterURL = "https://openrouter.ai/api/v1"
	}
	if strings.TrimSpace(normalized.OpenRouterModel) == "" {
		normalized.OpenRouterModel = "openai/gpt-4o-mini"
	}
	if strings.TrimSpace(normalized.OpenRouterKeyEnv) == "" {
		normalized.OpenRouterKeyEnv = "OPENROUTER_API_KEY"
	}
	if strings.TrimSpace(normalized.OllamaURL) == "" {
		normalized.OllamaURL = "http://127.0.0.1:11434"
	}
	if strings.TrimSpace(normalized.OllamaModel) == "" {
		normalized.OllamaModel = "llama3"
	}
	if strings.TrimSpace(normalized.LMStudioURL) == "" {
		normalized.LMStudioURL = "http://localhost:1234"
	}
	if strings.TrimSpace(normalized.LMStudioKeyEnv) == "" {
		normalized.LMStudioKeyEnv = "LMSTUDIO_API_KEY"
	}
	if normalized.BudgetExecute <= 0 {
		normalized.BudgetExecute = 120000
	}
	if normalized.BudgetReview <= 0 {
		normalized.BudgetReview = 80000
	}
	if normalized.StallWindow <= 0 {
		normalized.StallWindow = 3
	}
	if normalized.StallThreshold <= 0 {
		normalized.StallThreshold = 0.67
	}
	if normalized.StallThreshold > 1 {
		return domain.RunnerOptions{}, errors.New("stall threshold must be <= 1")
	}
	if normalized.RunnerMode == domain.RunnerTMUX {
		if !isTMUXCompatibleAgent(normalized.DefaultExecutor) {
			return domain.RunnerOptions{}, fmt.Errorf("runner mode %q does not support executor %q", normalized.RunnerMode, normalized.DefaultExecutor)
		}
		if !isTMUXCompatibleAgent(normalized.DefaultReviewer) {
			return domain.RunnerOptions{}, fmt.Errorf("runner mode %q does not support reviewer %q", normalized.RunnerMode, normalized.DefaultReviewer)
		}
		if strings.TrimSpace(normalized.Objective) != "" && !isTMUXCompatibleAgent(normalized.PlannerAgent) {
			return domain.RunnerOptions{}, fmt.Errorf("runner mode %q does not support planner %q", normalized.RunnerMode, normalized.PlannerAgent)
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

func mergeRunnerOptionsWithPlan(options domain.RunnerOptions, plan domain.Plan) (domain.RunnerOptions, error) {
	merged := options

	if !options.ExecutorAgentSet {
		if value := domain.NormalizeAgent(plan.Settings.Agents.Executor.Agent); value != "" {
			merged.DefaultExecutor = value
		}
	}
	if !options.ReviewerAgentSet {
		if value := domain.NormalizeAgent(plan.Settings.Agents.Reviewer.Agent); value != "" {
			merged.DefaultReviewer = value
		}
	}
	if !options.PlannerAgentSet {
		if value := domain.NormalizeAgent(plan.Settings.Agents.Planner.Agent); value != "" {
			merged.PlannerAgent = value
		}
	}
	if !options.ExecutorModelSet {
		if value := strings.TrimSpace(plan.Settings.Agents.Executor.Model); value != "" {
			merged.ExecutorModel = value
		}
	}
	if !options.ReviewerModelSet {
		if value := strings.TrimSpace(plan.Settings.Agents.Reviewer.Model); value != "" {
			merged.ReviewerModel = value
		}
	}
	if !options.PlannerModelSet {
		if value := strings.TrimSpace(plan.Settings.Agents.Planner.Model); value != "" {
			merged.PlannerModel = value
		}
	}

	policy := plan.Settings.ExecutionPolicy
	if !options.MaxRetriesSet && policy.MaxRetriesPerTask > 0 {
		merged.MaxRetries = policy.MaxRetriesPerTask
	}
	if !options.MaxIterationsSet && policy.MaxTotalIterations > 0 {
		merged.MaxIterations = policy.MaxTotalIterations
	}
	if !options.TimeoutSet && strings.TrimSpace(policy.Timeout) != "" {
		timeout, err := time.ParseDuration(strings.TrimSpace(policy.Timeout))
		if err != nil {
			return domain.RunnerOptions{}, fmt.Errorf("invalid plan timeout: %w", err)
		}
		merged.Timeout = timeout
	}
	if !options.BudgetExecuteSet && policy.Budget.Execute > 0 {
		merged.BudgetExecute = policy.Budget.Execute
	}
	if !options.BudgetReviewSet && policy.Budget.Review > 0 {
		merged.BudgetReview = policy.Budget.Review
	}
	if !options.StallDetectionSet {
		merged.StallDetection = policy.StallDetection.Enabled
	}
	if !options.StallWindowSet && policy.StallDetection.Window > 0 {
		merged.StallWindow = policy.StallDetection.Window
	}
	if !options.StallThresholdSet && policy.StallDetection.Threshold > 0 {
		merged.StallThreshold = policy.StallDetection.Threshold
	}

	return normalizeRunnerOptions(merged)
}

func validateRequiredBinaries(opts domain.RunnerOptions, plan domain.Plan) error {
	neededAgents := map[domain.Agent]struct{}{}
	if opts.RunnerMode == domain.RunnerTMUX {
		executor := domain.NormalizeAgent(opts.DefaultExecutor)
		if executor != "" && !isTMUXCompatibleAgent(executor) {
			return fmt.Errorf("runner mode %q does not support default executor %q; use --runner direct or --runner pty", opts.RunnerMode, executor)
		}
		if !opts.SkipReview {
			reviewer := domain.NormalizeAgent(opts.DefaultReviewer)
			if reviewer != "" && !isTMUXCompatibleAgent(reviewer) {
				return fmt.Errorf("runner mode %q does not support default reviewer %q; use --runner direct or --runner pty", opts.RunnerMode, reviewer)
			}
		}
	}

	if strings.TrimSpace(opts.Objective) != "" {
		neededAgents[domain.NormalizeAgent(opts.PlannerAgent)] = struct{}{}
	}
	neededAgents[domain.NormalizeAgent(opts.DefaultExecutor)] = struct{}{}
	if !opts.SkipReview {
		neededAgents[domain.NormalizeAgent(opts.DefaultReviewer)] = struct{}{}
	}

	neededBins := map[string]string{}
	var missing []string
	for agent := range neededAgents {
		agent = domain.NormalizeAgent(agent)
		if agent == "" || agent == domain.AgentNone {
			continue
		}

		if domain.AgentRequiresBinary(agent) {
			bin, ok := domain.AgentBinary(opts, agent)
			if !ok || strings.TrimSpace(bin) == "" {
				missing = append(missing, fmt.Sprintf("%s(binary not configured)", domain.AgentDisplayName(agent)))
				continue
			}
			if label, exists := neededBins[bin]; exists {
				if strings.Contains(label, string(agent)) {
					continue
				}
				neededBins[bin] = label + "+" + string(agent)
				continue
			}
			neededBins[bin] = string(agent)
			continue
		}

		if agent == domain.AgentOpenRouter {
			keyEnv := strings.TrimSpace(opts.OpenRouterKeyEnv)
			if keyEnv == "" {
				keyEnv = "OPENROUTER_API_KEY"
			}
			if strings.TrimSpace(os.Getenv(keyEnv)) == "" {
				missing = append(missing, fmt.Sprintf("%s(api key env %s not set)", domain.AgentDisplayName(agent), keyEnv))
			}
		}
	}

	for bin, label := range neededBins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, fmt.Sprintf("%s (%s)", label, bin))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("required provider prerequisites not met: %s", strings.Join(missing, ", "))
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

func resolveExecutor(defaultExecutor domain.Agent) (domain.Agent, error) {
	executor := domain.NormalizeAgent(defaultExecutor)
	if executor == domain.AgentNone {
		return "", errors.New("executor cannot be none")
	}
	if _, ok := domain.ValidExecutors[executor]; !ok {
		return "", fmt.Errorf("invalid executor %q", executor)
	}
	return executor, nil
}

func resolveReviewer(defaultReviewer domain.Agent, skipReview bool) (domain.Agent, error) {
	if skipReview {
		return domain.AgentNone, nil
	}

	reviewer := domain.NormalizeAgent(defaultReviewer)
	if _, ok := domain.ValidReviewers[reviewer]; !ok {
		return "", fmt.Errorf("invalid reviewer %q", reviewer)
	}
	return reviewer, nil
}

func isTMUXCompatibleAgent(agent domain.Agent) bool {
	return domain.AgentSupportsTMUX(agent)
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

func enrichGeneratedPlan(plan domain.Plan, opts domain.RunnerOptions) domain.Plan {
	if strings.TrimSpace(plan.Name) == "" {
		plan.Name = "generated plan"
	}
	if strings.TrimSpace(plan.Meta.Source) == "" {
		plan.Meta.Source = "agent"
	}
	if strings.TrimSpace(plan.Meta.CreatedAt) == "" {
		plan.Meta.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	executor := domain.NormalizeAgent(plan.Settings.Agents.Executor.Agent)
	if executor == "" {
		plan.Settings.Agents.Executor.Agent = domain.NormalizeAgent(opts.DefaultExecutor)
	}
	if strings.TrimSpace(plan.Settings.Agents.Executor.Model) == "" {
		plan.Settings.Agents.Executor.Model = strings.TrimSpace(opts.ExecutorModel)
	}

	reviewer := domain.NormalizeAgent(plan.Settings.Agents.Reviewer.Agent)
	if reviewer == "" {
		plan.Settings.Agents.Reviewer.Agent = domain.NormalizeAgent(opts.DefaultReviewer)
	}
	if strings.TrimSpace(plan.Settings.Agents.Reviewer.Model) == "" {
		plan.Settings.Agents.Reviewer.Model = strings.TrimSpace(opts.ReviewerModel)
	}

	planner := domain.NormalizeAgent(plan.Settings.Agents.Planner.Agent)
	if planner == "" {
		plan.Settings.Agents.Planner.Agent = domain.NormalizeAgent(opts.PlannerAgent)
	}
	if strings.TrimSpace(plan.Settings.Agents.Planner.Model) == "" {
		plan.Settings.Agents.Planner.Model = strings.TrimSpace(opts.PlannerModel)
	}

	policy := plan.Settings.ExecutionPolicy
	if policy.MaxRetriesPerTask <= 0 {
		policy.MaxRetriesPerTask = opts.MaxRetries
	}
	if policy.MaxTotalIterations < 0 {
		policy.MaxTotalIterations = 0
	}
	if strings.TrimSpace(policy.Timeout) == "" && opts.Timeout > 0 {
		policy.Timeout = opts.Timeout.String()
	}
	if policy.Budget.Execute <= 0 {
		policy.Budget.Execute = opts.BudgetExecute
	}
	if policy.Budget.Review <= 0 {
		policy.Budget.Review = opts.BudgetReview
	}
	if policy.StallDetection.Window <= 0 {
		policy.StallDetection.Window = opts.StallWindow
	}
	if policy.StallDetection.Threshold <= 0 {
		policy.StallDetection.Threshold = opts.StallThreshold
	}
	if !policy.StallDetection.Enabled {
		policy.StallDetection.Enabled = opts.StallDetection
	}
	plan.Settings.ExecutionPolicy = policy

	for i := range plan.Tasks {
		plan.Tasks[i].Acceptance = normalizeAcceptanceItems(plan.Tasks[i].Acceptance)
	}

	return plan
}

func normalizeAcceptanceItems(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	return normalized
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
		RunID:             run.runID,
		PlanSlug:          run.slug,
		PlanChecksum:      run.state.PlanChecksum,
		ProjectRoot:       run.projectRoot,
		ManifestPath:      run.manifestPath,
		ManifestHash:      run.manifestHash,
		ManifestTruncated: run.manifestTruncated,
		Phase:             strings.TrimSpace(phase),
		Message:           strings.TrimSpace(message),
		Outcome:           run.stats.Outcome,
		Iteration:         run.stats.Iterations,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		State:             run.state,
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

func binaryOverridesFromOpts(opts domain.RunnerOptions) map[agent.ID]string {
	m := map[agent.ID]string{}
	if opts.CodexBin != "" {
		m[agent.Codex] = opts.CodexBin
	}
	if opts.ClaudeBin != "" {
		m[agent.Claude] = opts.ClaudeBin
	}
	if opts.CopilotBin != "" {
		m[agent.Copilot] = opts.CopilotBin
	}
	if opts.GeminiBin != "" {
		m[agent.Gemini] = opts.GeminiBin
	}
	if opts.KimiBin != "" {
		m[agent.Kimi] = opts.KimiBin
	}
	if opts.OpenCodeBin != "" {
		m[agent.OpenCode] = opts.OpenCodeBin
	}
	return m
}

func restEndpointsFromOpts(opts domain.RunnerOptions) map[agent.ID]string {
	m := map[agent.ID]string{}
	if opts.OpenRouterURL != "" {
		m[agent.OpenRouter] = opts.OpenRouterURL
	}
	if opts.OllamaURL != "" {
		m[agent.Ollama] = opts.OllamaURL
	}
	if opts.LMStudioURL != "" {
		m[agent.LMStudio] = opts.LMStudioURL
	}
	return m
}
