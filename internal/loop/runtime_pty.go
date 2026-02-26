package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	ptyruntime "github.com/opus-domini/praetor/internal/runtime/pty"
)

// ptyRunner executes commands attached to a pseudo-terminal (PTY).
// Useful for tools that require an interactive TTY to behave correctly.
type ptyRunner struct{}

func (r *ptyRunner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error) {
	if len(spec.Args) == 0 {
		return ProcessResult{}, errors.New("empty command")
	}

	session := ptyruntime.NewScriptSession()
	if err := session.Start(ctx, ptyruntime.CommandSpec{
		Args: spec.Args,
		Dir:  spec.Dir,
		Env:  spec.Env,
	}); err != nil {
		return ProcessResult{}, err
	}

	if strings.TrimSpace(spec.Stdin) != "" {
		if err := session.Write(spec.Stdin); err != nil {
			_ = session.Close()
			return ProcessResult{}, err
		}
		if !strings.HasSuffix(spec.Stdin, "\n") {
			if err := session.Write("\n"); err != nil {
				_ = session.Close()
				return ProcessResult{}, err
			}
		}
		_ = session.CloseInput()
	}

	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder
	for ev := range session.Events() {
		switch ev.Source {
		case ptyruntime.StreamStdout:
			stdoutBuilder.WriteString(ev.Data)
		case ptyruntime.StreamStderr:
			stderrBuilder.WriteString(ev.Data)
		}
	}

	waitErr := session.Wait()
	if ctx.Err() != nil {
		return ProcessResult{}, ctx.Err()
	}

	stdoutText := strings.TrimSpace(stdoutBuilder.String())
	stderrText := strings.TrimSpace(stderrBuilder.String())
	if runDir != "" {
		if prefix == "" {
			prefix = "agent"
		}
		_ = os.WriteFile(filepath.Join(runDir, prefix+".stdout"), []byte(stdoutText), 0o644)
		_ = os.WriteFile(filepath.Join(runDir, prefix+".stderr"), []byte(stderrText), 0o644)
	}

	return ProcessResult{
		Stdout:   stdoutText,
		Stderr:   stderrText,
		ExitCode: session.ExitCode(),
	}, waitErr
}
