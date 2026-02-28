package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecCommandRunnerUsesProcessForNonTTYCommand(t *testing.T) {
	t.Parallel()

	runner := NewExecCommandRunner()
	result, err := runner.Run(context.Background(), CommandSpec{
		Args: []string{"sh", "-c", "echo ok"},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Strategy != "process" {
		t.Fatalf("expected process strategy, got %q", result.Strategy)
	}
	if result.Stdout != "ok" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
}

func TestExecCommandRunnerFallsBackToPTYOnTTYRequirement(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("pty fallback test requires Unix shell and script utility")
	}

	scriptPath := filepath.Join(t.TempDir(), "needs_tty.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nif [ ! -t 0 ]; then\n  echo 'stdin is not a tty' 1>&2\n  exit 1\nfi\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runner := NewExecCommandRunner()
	result, err := runner.Run(context.Background(), CommandSpec{
		Args: []string{"sh", scriptPath},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if result.Strategy != "pty" {
		t.Fatalf("expected pty strategy, got %q", result.Strategy)
	}
}

func TestExecCommandRunnerDisablePTYPreventsFallback(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("pty fallback test requires Unix shell and script utility")
	}

	scriptPath := filepath.Join(t.TempDir(), "needs_tty.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho 'stdin is not a tty' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runner := NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{DisablePTY: true})
	result, err := runner.Run(context.Background(), CommandSpec{
		Args: []string{"sh", scriptPath},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("expected non-zero exit when PTY fallback is disabled")
	}
	if result.Strategy != "process" {
		t.Fatalf("expected process strategy, got %q", result.Strategy)
	}
}
