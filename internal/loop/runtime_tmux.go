package loop

import (
	"context"

	tmuxruntime "github.com/opus-domini/praetor/internal/runtime/tmux"
)

// tmuxRunner delegates to runtime/tmux.Runner.
// It implements ProcessRunner and SessionManager.
type tmuxRunner struct {
	inner *tmuxruntime.Runner
}

func newTmuxRunner(sessionName string) *tmuxRunner {
	return &tmuxRunner{inner: tmuxruntime.NewRunner(sessionName)}
}

// SessionName returns the tmux session target used by this runner.
func (r *tmuxRunner) SessionName() string {
	return r.inner.SessionName()
}

// EnsureSession validates tmux availability and creates the target session if needed.
func (r *tmuxRunner) EnsureSession() error {
	return r.inner.EnsureSession()
}

// Cleanup kills the tmux session if this runner created it.
func (r *tmuxRunner) Cleanup() {
	r.inner.Cleanup()
}

// Run executes a CommandSpec in a dedicated tmux window.
func (r *tmuxRunner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error) {
	return r.inner.Run(ctx, spec, runDir, prefix)
}

// buildTmuxWrapperScript delegates to the tmux package.
func buildTmuxWrapperScript(spec CommandSpec, stdinFile, stdoutFile, stderrFile, exitFile, channel string) string {
	return tmuxruntime.BuildWrapperScript(spec, stdinFile, stdoutFile, stderrFile, exitFile, channel)
}

// tmuxWindowName delegates to the tmux package.
func tmuxWindowName(taskLabel, role string) string {
	return tmuxruntime.WindowName(taskLabel, role)
}

// killTMUXWindow delegates to the tmux package.
func killTMUXWindow(windowID string) {
	tmuxruntime.KillWindow(windowID)
}

// shellQuote delegates to the tmux package.
func shellQuote(value string) string {
	return tmuxruntime.ShellQuote(value)
}

// tailText delegates to the tmux package.
func tailText(value string, maxLines int) string {
	return tmuxruntime.TailText(value, maxLines)
}
