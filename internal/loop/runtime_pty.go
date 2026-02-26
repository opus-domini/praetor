package loop

import (
	"context"

	ptyruntime "github.com/opus-domini/praetor/internal/runtime/pty"
)

// ptyRunner delegates to runtime/pty.Runner.
type ptyRunner struct {
	inner ptyruntime.Runner
}

func (r *ptyRunner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error) {
	return r.inner.Run(ctx, spec, runDir, prefix)
}
