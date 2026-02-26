package agents

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestExecCommandRunnerUsesProcessForNonTTYCommand(t *testing.T) {
	t.Parallel()

	runner := NewExecCommandRunner()
	result, err := runner.Run(context.Background(), CommandSpec{
		Args:   []string{"/bin/sh", "-lc", "echo hello"},
		UsePTY: false,
	})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if got := strings.TrimSpace(result.Stdout); got != "hello" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.Strategy != "process" {
		t.Fatalf("expected process strategy, got %q", result.Strategy)
	}
}

func TestExecCommandRunnerFallsBackToPTYOnTTYRequirement(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script command not available")
	}
	ttyBin, err := exec.LookPath("tty")
	if err != nil {
		t.Skip("tty command not available")
	}

	runner := NewExecCommandRunner()
	result, err := runner.Run(context.Background(), CommandSpec{
		Args:   []string{ttyBin},
		UsePTY: false,
	})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0 after PTY fallback, got %d (stderr=%q)", result.ExitCode, result.Stderr)
	}
	if result.Strategy != "pty" {
		t.Fatalf("expected pty strategy after fallback, got %q", result.Strategy)
	}
	if !strings.Contains(result.Stdout, "/") {
		t.Fatalf("expected tty path in stdout, got %q", result.Stdout)
	}
}

func TestExecCommandRunnerDisablePTYPreventsFallback(t *testing.T) {
	t.Parallel()

	ttyBin, err := exec.LookPath("tty")
	if err != nil {
		t.Skip("tty command not available")
	}

	runner := NewExecCommandRunnerWithOptions(ExecCommandRunnerOptions{DisablePTY: true})
	result, err := runner.Run(context.Background(), CommandSpec{
		Args:   []string{ttyBin},
		UsePTY: false,
	})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if result.Strategy != "process" {
		t.Fatalf("expected process strategy, got %q", result.Strategy)
	}
	if result.ExitCode == 0 {
		t.Fatalf("expected non-zero exit code without PTY fallback")
	}
}
