package loop

import (
	"context"

	"github.com/opus-domini/praetor/internal/runtime/process"
)

// directRunner delegates to runtime/process.Runner.
type directRunner struct {
	inner process.Runner
}

func (r *directRunner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error) {
	return r.inner.Run(ctx, spec, runDir, prefix)
}
