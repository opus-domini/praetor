package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
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

func TestExecCommandRunnerStripsNestingEnvVars(t *testing.T) {
	// Set all nesting vars — child process must NOT see them.
	for _, name := range domain.AgentNestingEnvVars {
		t.Setenv(name, "1")
	}

	// Build a shell command that prints each nesting var's value.
	// If stripping works, all values should be empty.
	var checks []string
	for _, name := range domain.AgentNestingEnvVars {
		checks = append(checks, "echo "+name+"=${"+name+":-}")
	}
	script := strings.Join(checks, "; ")

	runner := NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{DisablePTY: true})
	result, err := runner.Run(context.Background(), CommandSpec{
		Args: []string{"sh", "-c", script},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%q", result.ExitCode, result.Stderr)
	}

	for _, name := range domain.AgentNestingEnvVars {
		expected := name + "="
		if !strings.Contains(result.Stdout, expected) {
			t.Errorf("expected %q in stdout (var should be empty), got: %s", expected, result.Stdout)
		}
		notExpected := name + "=1"
		if strings.Contains(result.Stdout, notExpected) {
			t.Errorf("nesting var %s leaked to child process", name)
		}
	}
}

func TestExecCommandRunnerPreservesSpecEnv(t *testing.T) {
	t.Parallel()

	runner := NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{DisablePTY: true})
	result, err := runner.Run(context.Background(), CommandSpec{
		Args: []string{"sh", "-c", "echo PRAETOR_TEST=$PRAETOR_TEST"},
		Env:  []string{"PRAETOR_TEST=hello"},
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "PRAETOR_TEST=hello") {
		t.Errorf("spec env var not passed to child: %s", result.Stdout)
	}
}
