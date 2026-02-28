package process

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// Runner executes commands as direct subprocesses without any TTY or tmux wrapping.
// It implements domain.ProcessRunner.
type Runner struct{}

// Run executes the command described by spec and returns its output.
// If runDir and prefix are non-empty, stdout and stderr are persisted as files.
func (r *Runner) Run(ctx context.Context, spec domain.CommandSpec, runDir, prefix string) (domain.ProcessResult, error) {
	if len(spec.Args) == 0 {
		return domain.ProcessResult{}, errors.New("empty command")
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

	return domain.ProcessResult{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
	}, err
}
