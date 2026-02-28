package runner

import (
	"context"
	"errors"

	"github.com/opus-domini/praetor/internal/domain"
)

// processRunnerAdapter adapts domain.ProcessRunner to the CommandRunner contract.
type processRunnerAdapter struct {
	runner   domain.ProcessRunner
	strategy string
}

// NewFromProcessRunner creates a command runner from a domain.ProcessRunner.
func NewFromProcessRunner(runner domain.ProcessRunner, strategy string) CommandRunner {
	return &processRunnerAdapter{runner: runner, strategy: strategy}
}

func (r *processRunnerAdapter) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	if r == nil || r.runner == nil {
		return CommandResult{}, errors.New("process runner is not configured")
	}
	result, err := r.runner.Run(ctx, domain.CommandSpec{
		Args:       spec.Args,
		Env:        spec.Env,
		Dir:        spec.Dir,
		Stdin:      spec.Stdin,
		WindowHint: spec.WindowHint,
	}, spec.RunDir, spec.OutputPrefix)
	return CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Strategy: r.strategy,
	}, err
}
