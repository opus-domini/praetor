package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CommandSpec describes a process invocation.
type CommandSpec struct {
	Args  []string // Full command
	Env   []string // Additional environment variables (KEY=VALUE)
	Dir   string   // Working directory
	Stdin string   // Content to write to stdin ("" = no stdin)
}

// Result holds the raw output of a completed process.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes commands as direct subprocesses without any TTY or tmux wrapping.
type Runner struct{}

// Run executes the command described by spec and returns its output.
// If runDir and prefix are non-empty, stdout and stderr are persisted as files.
func (r *Runner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (Result, error) {
	if len(spec.Args) == 0 {
		return Result{}, errors.New("empty command")
	}

	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}

	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not a Go error
		}
	}

	// Persist to files for debugging.
	if runDir != "" {
		if prefix == "" {
			prefix = "agent"
		}
		_ = os.WriteFile(filepath.Join(runDir, prefix+".stdout"), []byte(stdout.String()), 0o644)
		_ = os.WriteFile(filepath.Join(runDir, prefix+".stderr"), []byte(stderr.String()), 0o644)
	}

	return Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
	}, err
}
