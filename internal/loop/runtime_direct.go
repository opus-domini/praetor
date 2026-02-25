package loop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// directRunner executes commands as direct subprocesses.
// Useful for testing and for future non-tmux execution.
type directRunner struct{}

func (r *directRunner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error) {
	if len(spec.Args) == 0 {
		return ProcessResult{}, errors.New("empty command")
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

	// Persist to files for debugging (same contract as tmux).
	if runDir != "" {
		if prefix == "" {
			prefix = "agent"
		}
		_ = os.WriteFile(filepath.Join(runDir, prefix+".stdout"), []byte(stdout.String()), 0o644)
		_ = os.WriteFile(filepath.Join(runDir, prefix+".stderr"), []byte(stderr.String()), 0o644)
	}

	return ProcessResult{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
	}, err
}
