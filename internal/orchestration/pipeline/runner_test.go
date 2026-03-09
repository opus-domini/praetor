package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

func TestNormalizeRunnerOptionsRejectsInvalidMaxRetries(t *testing.T) {
	t.Parallel()

	_, err := normalizeRunnerOptions(domain.RunnerOptions{
		ProjectHome:     t.TempDir(),
		Workdir:         t.TempDir(),
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentClaude,
		MaxRetries:      0,
		Isolation:       domain.IsolationOff,
	})
	if err == nil {
		t.Fatal("expected max retries validation error")
	}
}

func TestNormalizeRunnerOptionsSetsDefaultGeminiAndOllamaSettings(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	normalized, err := normalizeRunnerOptions(domain.RunnerOptions{
		ProjectHome:     t.TempDir(),
		Workdir:         workdir,
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentClaude,
		MaxRetries:      1,
		RunnerMode:      domain.RunnerDirect,
		Isolation:       domain.IsolationOff,
	})
	if err != nil {
		t.Fatalf("normalize options: %v", err)
	}
	if normalized.GeminiBin != "gemini" {
		t.Fatalf("expected default gemini binary, got %q", normalized.GeminiBin)
	}
	if normalized.CopilotBin != "copilot" {
		t.Fatalf("expected default copilot binary, got %q", normalized.CopilotBin)
	}
	if normalized.KimiBin != "kimi" {
		t.Fatalf("expected default kimi binary, got %q", normalized.KimiBin)
	}
	if normalized.OpenCodeBin != "opencode" {
		t.Fatalf("expected default opencode binary, got %q", normalized.OpenCodeBin)
	}
	if normalized.OpenRouterURL == "" {
		t.Fatal("expected default openrouter url")
	}
	if normalized.OpenRouterModel == "" {
		t.Fatal("expected default openrouter model")
	}
	if normalized.OpenRouterKeyEnv == "" {
		t.Fatal("expected default openrouter key env")
	}
	if normalized.OllamaURL == "" {
		t.Fatal("expected default ollama url")
	}
	if normalized.OllamaModel == "" {
		t.Fatal("expected default ollama model")
	}
	if normalized.ExecutorPromptChars != 120000 {
		t.Fatalf("expected default executor prompt chars, got %d", normalized.ExecutorPromptChars)
	}
	if normalized.ReviewerPromptChars != 80000 {
		t.Fatalf("expected default reviewer prompt chars, got %d", normalized.ReviewerPromptChars)
	}
	if normalized.CostWarnThreshold != 0.80 {
		t.Fatalf("expected default cost warn threshold, got %.2f", normalized.CostWarnThreshold)
	}
	if !normalized.CostBudgetEnforce {
		t.Fatal("expected cost budget enforcement to default to true")
	}
}

func TestBuildAgentRuntimeUsesRegistryOutsideTMUX(t *testing.T) {
	t.Parallel()

	runtime, err := BuildAgentRuntime(domain.RunnerOptions{
		RunnerMode:  domain.RunnerDirect,
		CodexBin:    "codex",
		ClaudeBin:   "claude",
		GeminiBin:   "gemini",
		OllamaURL:   "http://127.0.0.1:11434",
		OllamaModel: "llama3",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	if runtime == nil {
		t.Fatal("expected non-nil runtime")
	}
}

func TestValidateRequiredBinariesSkipsReviewerWhenNoReview(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve executable: %v", err)
	}

	opts := domain.RunnerOptions{
		SkipReview:      true,
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentClaude,
		CodexBin:        self,
		ClaudeBin:       "__missing_claude_binary__",
	}
	plan := testPlan([]domain.Task{
		{ID: "TASK-001", Title: "task", Acceptance: []string{"command succeeds"}},
	})
	if err := validateRequiredBinaries(opts, plan); err != nil {
		t.Fatalf("expected reviewer checks to be skipped, got error: %v", err)
	}
}

func TestValidateRequiredBinariesRequiresReviewerWhenEnabled(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve executable: %v", err)
	}

	opts := domain.RunnerOptions{
		SkipReview:      false,
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentClaude,
		CodexBin:        self,
		ClaudeBin:       "__missing_claude_binary__",
	}
	plan := testPlan([]domain.Task{
		{ID: "TASK-001", Title: "task", Acceptance: []string{"command succeeds"}},
	})
	err = validateRequiredBinaries(opts, plan)
	if err == nil {
		t.Fatal("expected missing reviewer binary error")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRequiredBinariesOpenRouterRequiresAPIKey(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve executable: %v", err)
	}

	opts := domain.RunnerOptions{
		SkipReview:       true,
		DefaultExecutor:  domain.AgentOpenRouter,
		DefaultReviewer:  domain.AgentNone,
		CodexBin:         self,
		ClaudeBin:        self,
		OpenRouterKeyEnv: "PRAETOR_TEST_OPENROUTER_KEY",
	}
	plan := testPlan([]domain.Task{
		{ID: "TASK-001", Title: "task", Acceptance: []string{"command succeeds"}},
	})
	t.Setenv("PRAETOR_TEST_OPENROUTER_KEY", "")

	err = validateRequiredBinaries(opts, plan)
	if err == nil {
		t.Fatal("expected missing openrouter api key validation error")
	}
	if !strings.Contains(err.Error(), "PRAETOR_TEST_OPENROUTER_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPostTaskHookCapturesStderr(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	hookPath := filepath.Join(tmpDir, "hook.sh")
	script := "#!/bin/sh\necho 'hook stderr output' 1>&2\nexit 1\n"
	if err := os.WriteFile(hookPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	ok, feedback := runPostTaskHook(context.Background(), hookPath, tmpDir, tmpDir)
	if ok {
		t.Fatal("expected hook to fail")
	}
	// Feedback can fallback to a generic message when the command fails before
	// stderr is surfaced through exec buffers on some environments.
	if feedback != "post-task hook failed" && !strings.Contains(feedback, "hook stderr output") {
		t.Fatalf("unexpected feedback: %q", feedback)
	}

	stderrData, err := os.ReadFile(filepath.Join(tmpDir, "post-hook.stderr"))
	if err != nil {
		t.Fatalf("read post-hook.stderr: %v", err)
	}
	if !strings.Contains(string(stderrData), "hook stderr output") {
		t.Fatalf("expected stderr file to contain process stderr, got %q", string(stderrData))
	}
}

func TestRunnerStopsImmediatelyOnCanceledContext(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	projectHome := filepath.Join(tmpDir, "home")
	slug := "test-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task", Acceptance: []string{"task completes"}},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewRunner(nilRuntime{})
	_, err := runner.Run(ctx, discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:      projectHome,
		Workdir:          tmpDir,
		DefaultExecutor:  domain.AgentCodex,
		DefaultReviewer:  domain.AgentNone,
		MaxRetries:       3,
		MaxParallelTasks: 1,
		SkipReview:       true,
		CodexBin:         mustExecutablePath(t),
		ClaudeBin:        "__missing_claude_binary__",
		TMUXSession:      "test-session",
		Isolation:        domain.IsolationOff,
	})
	if err == nil {
		t.Fatal("expected canceled context error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunnerReleasesLockWhenBootstrapFailsAfterLock(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	projectHome := filepath.Join(tmpDir, "home")
	slug := "test-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task", Acceptance: []string{"task completes"}},
	}))

	lockPath := store.LockFile(slug)
	runner := NewRunner(nilRuntime{})
	_, err := runner.Run(context.Background(), discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:     projectHome,
		Workdir:         tmpDir, // not a git repo -> prune orphans fails after lock acquisition
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentNone,
		MaxRetries:      3,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       mustExecutablePath(t),
		Isolation:       domain.IsolationWorktree,
	})
	if err == nil {
		t.Fatal("expected bootstrap failure in non-git workdir")
	}
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected lock file to be released on bootstrap failure, stat err=%v", statErr)
	}
}

func TestRunnerFailsWhenMaxTransitionsExceeded(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	git(t, tmpDir, "init")

	projectHome := filepath.Join(tmpDir, "home")
	slug := "test-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task", Acceptance: []string{"task completes"}},
	}))

	runner := NewRunner(scriptedRuntime{
		executeOutput: "RESULT: PASS\nSUMMARY: ok",
		reviewOutput:  "PASS|ok",
	})
	_, err := runner.Run(context.Background(), discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:     projectHome,
		Workdir:         tmpDir,
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentNone,
		MaxRetries:      2,
		MaxTransitions:  1,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       mustExecutablePath(t),
		Isolation:       domain.IsolationOff,
		RunnerMode:      domain.RunnerDirect,
	})
	if err == nil {
		t.Fatal("expected max transitions error")
	}
	if !strings.Contains(err.Error(), "max transitions reached") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunnerReviewRejectionExhaustsRetries(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	git(t, tmpDir, "init")

	projectHome := filepath.Join(tmpDir, "home")
	slug := "test-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task", Acceptance: []string{"task completes"}},
	}))

	runner := NewRunner(scriptedRuntime{
		executeOutput: "RESULT: PASS\nSUMMARY: ok",
		reviewOutput:  "FAIL|missing tests",
	})
	_, err := runner.Run(context.Background(), discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:     projectHome,
		Workdir:         tmpDir,
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentClaude,
		MaxRetries:      1,
		SkipReview:      false,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       mustExecutablePath(t),
		Isolation:       domain.IsolationOff,
		RunnerMode:      domain.RunnerDirect,
	})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}

	state, readErr := store.ReadState(slug)
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	if got := state.Tasks[0].Status; got != domain.TaskFailed {
		t.Fatalf("expected failed task after retry exhaustion, got %s", got)
	}
}

func TestRunnerWritesRuntimeStrategyCheckpoint(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	git(t, tmpDir, "init")

	projectHome := filepath.Join(tmpDir, "home")
	slug := "test-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task", Acceptance: []string{"task completes"}},
	}))

	runner := NewRunner(scriptedRuntime{
		executeOutput: "RESULT: PASS\nSUMMARY: ok",
	})
	_, err := runner.Run(context.Background(), discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:     projectHome,
		Workdir:         tmpDir,
		DefaultExecutor: domain.AgentCodex,
		DefaultReviewer: domain.AgentNone,
		MaxRetries:      2,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       mustExecutablePath(t),
		Isolation:       domain.IsolationOff,
		RunnerMode:      domain.RunnerDirect,
	})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}

	historyPath := filepath.Join(projectHome, "checkpoints", "history.tsv")
	history, readErr := os.ReadFile(historyPath)
	if readErr != nil {
		t.Fatalf("read checkpoint history: %v", readErr)
	}
	if !strings.Contains(string(history), "\truntime_strategy\t") {
		t.Fatalf("expected runtime_strategy checkpoint entry, got:\n%s", string(history))
	}
}

func TestRunnerKeepsTaskOpenWhenMergeFails(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	git(t, tmpDir, "init")
	git(t, tmpDir, "config", "user.email", "test@example.com")
	git(t, tmpDir, "config", "user.name", "Praetor Test")

	conflictFile := filepath.Join(tmpDir, "conflict.txt")
	if err := os.WriteFile(conflictFile, []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	git(t, tmpDir, "add", "-A")
	git(t, tmpDir, "commit", "-m", "base commit")

	projectHome := filepath.Join(tmpDir, "home")
	slug := "test-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task", Acceptance: []string{"task completes"}},
	}))

	runtime := &mergeConflictRuntime{mainDir: tmpDir}
	runner := NewRunner(runtime)

	_, err := runner.Run(context.Background(), discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:      projectHome,
		Workdir:          tmpDir,
		DefaultExecutor:  domain.AgentCodex,
		DefaultReviewer:  domain.AgentNone,
		MaxRetries:       3,
		MaxParallelTasks: 1,
		SkipReview:       true,
		CodexBin:         mustExecutablePath(t),
		ClaudeBin:        mustExecutablePath(t),
		Isolation:        domain.IsolationWorktree,
	})
	if err == nil {
		t.Fatal("expected merge conflict error")
	}
	if !strings.Contains(err.Error(), "merge conflict") {
		t.Fatalf("unexpected error: %v", err)
	}

	state, readErr := store.ReadState(slug)
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	// After a merge conflict, the task stays in executing (the commit failed
	// before transition to done). On next load, crash recovery would reset
	// it to pending.
	if got := state.Tasks[0].Status; got != domain.TaskExecuting {
		t.Fatalf("expected task to remain executing after merge conflict, got %s", got)
	}
}

func TestRunnerExecutesIndependentTasksInParallelWaves(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	git(t, tmpDir, "init")

	projectHome := filepath.Join(tmpDir, "home")
	slug := "parallel-plan"
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	writePlanFile(t, store.PlanFile(slug), testPlan([]domain.Task{
		{ID: "TASK-001", Title: "Task 1", Acceptance: []string{"done"}},
		{ID: "TASK-002", Title: "Task 2", Acceptance: []string{"done"}},
		{ID: "TASK-003", Title: "Task 3", DependsOn: []string{"TASK-001", "TASK-002"}, Acceptance: []string{"done"}},
	}))

	runtime := &parallelProbeRuntime{releaseFirstWave: make(chan struct{})}
	runner := NewRunner(runtime)
	_, err := runner.Run(context.Background(), discardSink{}, slug, domain.RunnerOptions{
		ProjectHome:      projectHome,
		Workdir:          tmpDir,
		DefaultExecutor:  domain.AgentCodex,
		DefaultReviewer:  domain.AgentNone,
		MaxRetries:       2,
		MaxParallelTasks: 2,
		SkipReview:       true,
		CodexBin:         mustExecutablePath(t),
		ClaudeBin:        mustExecutablePath(t),
		Isolation:        domain.IsolationOff,
		RunnerMode:       domain.RunnerDirect,
	})
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}

	state, err := store.ReadState(slug)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state.DoneCount() != 3 {
		t.Fatalf("done count = %d, want 3", state.DoneCount())
	}
	if runtime.maxActive < 2 {
		t.Fatalf("max active executors = %d, want >= 2", runtime.maxActive)
	}
	if len(runtime.executeOrder) != 3 {
		t.Fatalf("execute order len = %d, want 3", len(runtime.executeOrder))
	}
	if runtime.executeOrder[2] != "TASK-003" {
		t.Fatalf("dependent task executed too early: %v", runtime.executeOrder)
	}
}

type nilRuntime struct{}

func (nilRuntime) Run(context.Context, domain.AgentRequest) (domain.AgentResult, error) {
	return domain.AgentResult{}, nil
}

type discardSink struct{}

func (discardSink) Header(string)                                 {}
func (discardSink) KV(string, string)                             {}
func (discardSink) Task(string, string, string)                   {}
func (discardSink) Phase(string, string, string)                  {}
func (discardSink) Info(string)                                   {}
func (discardSink) Success(string)                                {}
func (discardSink) Warn(string)                                   {}
func (discardSink) Error(string)                                  {}
func (discardSink) Summary(int, int, int, float64, time.Duration) {}

func mustExecutablePath(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve executable path: %v", err)
	}
	return path
}

type mergeConflictRuntime struct {
	mainDir string
	done    bool
}

type parallelProbeRuntime struct {
	mu               sync.Mutex
	active           int
	maxActive        int
	executeOrder     []string
	firstWaveStarted int
	releaseFirstWave chan struct{}
}

type scriptedRuntime struct {
	executeOutput string
	executeErr    error
	reviewOutput  string
	reviewErr     error
}

func (r scriptedRuntime) Run(_ context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	switch req.Role {
	case "execute":
		return domain.AgentResult{
			Output:    r.executeOutput,
			DurationS: 0.1,
			Strategy:  domain.ExecutionStrategyProcess,
		}, r.executeErr
	case "review":
		return domain.AgentResult{
			Output:    r.reviewOutput,
			DurationS: 0.1,
			Strategy:  domain.ExecutionStrategyProcess,
		}, r.reviewErr
	default:
		return domain.AgentResult{
			Output:    "RESULT: PASS\nSUMMARY: ok",
			DurationS: 0.1,
			Strategy:  domain.ExecutionStrategyStructured,
		}, nil
	}
}

func (r *mergeConflictRuntime) Run(_ context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	if req.Role != "execute" {
		return domain.AgentResult{Output: "PASS|ok"}, nil
	}

	target := filepath.Join(req.Workdir, "conflict.txt")
	if err := os.WriteFile(target, []byte("worktree\n"), 0o644); err != nil {
		return domain.AgentResult{}, err
	}

	if !r.done {
		mainFile := filepath.Join(r.mainDir, "conflict.txt")
		if err := os.WriteFile(mainFile, []byte("main\n"), 0o644); err != nil {
			return domain.AgentResult{}, err
		}
		if err := runGitCommand(r.mainDir, "add", "-A"); err != nil {
			return domain.AgentResult{}, err
		}
		if err := runGitCommand(r.mainDir, "commit", "-m", "main side update"); err != nil {
			return domain.AgentResult{}, err
		}
		r.done = true
	}

	return domain.AgentResult{Output: "RESULT: PASS\nSUMMARY: ok"}, nil
}

func (r *parallelProbeRuntime) Run(_ context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	if req.Role != "execute" {
		return domain.AgentResult{Output: "PASS|ok", DurationS: 0.1, Strategy: domain.ExecutionStrategyProcess}, nil
	}

	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.executeOrder = append(r.executeOrder, req.TaskLabel)
	if req.TaskLabel == "TASK-001" || req.TaskLabel == "TASK-002" {
		r.firstWaveStarted++
		if r.firstWaveStarted == 2 {
			close(r.releaseFirstWave)
		}
	}
	r.mu.Unlock()

	if req.TaskLabel == "TASK-001" || req.TaskLabel == "TASK-002" {
		<-r.releaseFirstWave
	}
	time.Sleep(50 * time.Millisecond)

	r.mu.Lock()
	r.active--
	r.mu.Unlock()

	return domain.AgentResult{
		Output:    "RESULT: PASS\nSUMMARY: ok",
		DurationS: 0.1,
		Strategy:  domain.ExecutionStrategyProcess,
	}, nil
}

func testPlan(tasks []domain.Task) domain.Plan {
	return domain.Plan{
		Name: "test plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: tasks,
	}
}

func writePlanFile(t *testing.T, path string, plan domain.Plan) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create plan dir: %v", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("encode plan: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	if err := runGitCommand(dir, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
