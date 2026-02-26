package agents

import (
	"context"
	"errors"

	"github.com/opus-domini/praetor/internal/domain"
)

// processAdapterCommandRunner adapts domain.ProcessRunner to the agents.CommandRunner contract.
type processAdapterCommandRunner struct {
	runner   domain.ProcessRunner
	strategy string
}

// NewProcessAdapterCommandRunner creates a command runner from a domain.ProcessRunner.
func NewProcessAdapterCommandRunner(runner domain.ProcessRunner, strategy string) CommandRunner {
	return &processAdapterCommandRunner{runner: runner, strategy: strategy}
}

func (r *processAdapterCommandRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
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
