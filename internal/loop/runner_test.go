package loop

import (
	"context"
	"os"
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
		GitSafety:       false,
	})
	if err == nil {
		t.Fatal("expected max retries validation error")
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
	if !strings.Contains(feedback, "hook stderr output") {
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
		Workdir:         tmpDir,
		DefaultExecutor: AgentCodex,
		DefaultReviewer: AgentNone,
		MaxRetries:      3,
		SkipReview:      true,
		CodexBin:        mustExecutablePath(t),
		ClaudeBin:       "__missing_claude_binary__",
		TMUXSession:     "test-session",
		GitSafety:       false,
		GitSafetyMode:   GitSafetyModeOff,
	})
	if err == nil {
		t.Fatal("expected canceled context error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("unexpected error: %v", err)
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
