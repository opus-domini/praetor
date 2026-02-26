package loop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRunnerOptionsRejectsInvalidMaxRetries(t *testing.T) {
	t.Parallel()

	_, err := normalizeRunnerOptions(RunnerOptions{
		StateRoot:       t.TempDir(),
		Workdir:         t.TempDir(),
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentClaude,
		MaxRetries:      0,
		Isolation:       IsolationOff,
	})
	if err == nil {
		t.Fatal("expected max retries validation error")
	}
}

func TestNormalizeRunnerOptionsSetsDefaultGeminiAndOllamaSettings(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	normalized, err := normalizeRunnerOptions(RunnerOptions{
		StateRoot:       t.TempDir(),
		CacheRoot:       t.TempDir(),
		Workdir:         workdir,
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentClaude,
		MaxRetries:      1,
		RunnerMode:      RunnerDirect,
		Isolation:       IsolationOff,
	})
	if err != nil {
		t.Fatalf("normalize options: %v", err)
	}
	if normalized.GeminiBin != "gemini" {
		t.Fatalf("expected default gemini binary, got %q", normalized.GeminiBin)
	}
	if normalized.OllamaURL == "" {
		t.Fatal("expected default ollama url")
	}
	if normalized.OllamaModel == "" {
		t.Fatal("expected default ollama model")
	}
}

func TestBuildAgentRuntimeUsesRegistryOutsideTMUX(t *testing.T) {
	t.Parallel()

	runtime, err := buildAgentRuntime(RunnerOptions{
		RunnerMode:  RunnerDirect,
		CodexBin:    "codex",
		ClaudeBin:   "claude",
		GeminiBin:   "gemini",
		OllamaURL:   "http://127.0.0.1:11434",
		OllamaModel: "llama3",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	if _, ok := runtime.(*registryRuntime); !ok {
		t.Fatalf("expected registry runtime, got %T", runtime)
	}
}

func TestValidateRequiredBinariesSkipsReviewerWhenNoReview(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve executable: %v", err)
	}

	opts := RunnerOptions{
		SkipReview:      true,
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentClaude,
		CodexBin:        self,
		ClaudeBin:       "__missing_claude_binary__",
	}
	plan := Plan{
		Tasks: []Task{
			{Title: "task", Executor: AgentCodex, Reviewer: AgentClaude},
		},
	}
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

	opts := RunnerOptions{
		SkipReview:      false,
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentClaude,
		CodexBin:        self,
		ClaudeBin:       "__missing_claude_binary__",
	}
	plan := Plan{
		Tasks: []Task{
			{Title: "task", Executor: AgentCodex, Reviewer: AgentClaude},
		},
	}
	err = validateRequiredBinaries(opts, plan)
	if err == nil {
		t.Fatal("expected missing reviewer binary error")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPostTaskHookCapturesStderr(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	hookPath := filepath.Join(tmpDir, "hook.sh")
	script := "#!/usr/bin/env bash\necho 'hook stderr output' 1>&2\nexit 1\n"
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
	planPath := filepath.Join(tmpDir, "plan.json")
	writePlanFile(t, planPath, Plan{
		Tasks: []Task{
			{ID: "TASK-001", Title: "Task", Executor: AgentCodex, Reviewer: AgentNone},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := NewRunner(nilRuntime{})
	_, err := runner.Run(ctx, ioDiscard{}, planPath, RunnerOptions{
		StateRoot:       filepath.Join(tmpDir, "state"),
		CacheRoot:       filepath.Join(tmpDir, "cache"),
		Workdir:         tmpDir,
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentNone,
		MaxRetries:      3,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       "__missing_claude_binary__",
		TMUXSession:     "test-session",
		Isolation:       IsolationOff,
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
	planPath := filepath.Join(tmpDir, "plan.json")
	writePlanFile(t, planPath, Plan{
		Tasks: []Task{
			{ID: "TASK-001", Title: "Task", Executor: AgentCodex, Reviewer: AgentNone},
		},
	})

	stateRoot := filepath.Join(tmpDir, "state")
	store := NewStore(stateRoot, stateRoot)
	lockPath := store.LockFile(planPath)

	runner := NewRunner(nilRuntime{})
	_, err := runner.Run(context.Background(), ioDiscard{}, planPath, RunnerOptions{
		StateRoot:       stateRoot,
		Workdir:         tmpDir, // not a git repo -> prune orphans fails after lock acquisition
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentNone,
		MaxRetries:      3,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       mustExecutablePath(t),
		Isolation:       IsolationWorktree,
	})
	if err == nil {
		t.Fatal("expected bootstrap failure in non-git workdir")
	}
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected lock file to be released on bootstrap failure, stat err=%v", statErr)
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

	planPath := filepath.Join(tmpDir, "plan.json")
	writePlanFile(t, planPath, Plan{
		Tasks: []Task{
			{ID: "TASK-001", Title: "Task", Executor: AgentCodex, Reviewer: AgentNone},
		},
	})

	runtime := &mergeConflictRuntime{mainDir: tmpDir}
	stateRoot := filepath.Join(tmpDir, "state")
	runner := NewRunner(runtime)

	_, err := runner.Run(context.Background(), ioDiscard{}, planPath, RunnerOptions{
		StateRoot:       stateRoot,
		Workdir:         tmpDir,
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentNone,
		MaxRetries:      3,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       mustExecutablePath(t),
		Isolation:       IsolationWorktree,
	})
	if err == nil {
		t.Fatal("expected merge conflict error")
	}
	if !strings.Contains(err.Error(), "merge conflict") {
		t.Fatalf("unexpected error: %v", err)
	}

	store := NewStore(stateRoot, stateRoot)
	state, readErr := store.ReadState(planPath)
	if readErr != nil {
		t.Fatalf("read state: %v", readErr)
	}
	// After a merge conflict, the task stays in executing (the commit failed
	// before transition to done). On next load, crash recovery would reset
	// it to pending.
	if got := state.Tasks[0].Status; got != TaskExecuting {
		t.Fatalf("expected task to remain executing after merge conflict, got %s", got)
	}
}

type nilRuntime struct{}

func (nilRuntime) Run(context.Context, AgentRequest) (AgentResult, error) {
	return AgentResult{}, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

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

func (r *mergeConflictRuntime) Run(_ context.Context, req AgentRequest) (AgentResult, error) {
	if req.Role != "execute" {
		return AgentResult{Output: "PASS|ok"}, nil
	}

	target := filepath.Join(req.Workdir, "conflict.txt")
	if err := os.WriteFile(target, []byte("worktree\n"), 0o644); err != nil {
		return AgentResult{}, err
	}

	if !r.done {
		mainFile := filepath.Join(r.mainDir, "conflict.txt")
		if err := os.WriteFile(mainFile, []byte("main\n"), 0o644); err != nil {
			return AgentResult{}, err
		}
		if err := runGitCommand(r.mainDir, "add", "-A"); err != nil {
			return AgentResult{}, err
		}
		if err := runGitCommand(r.mainDir, "commit", "-m", "main side update"); err != nil {
			return AgentResult{}, err
		}
		r.done = true
	}

	return AgentResult{Output: "RESULT: PASS\nSUMMARY: ok"}, nil
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
