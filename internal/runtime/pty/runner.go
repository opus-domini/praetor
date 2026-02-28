package pty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// Runner executes commands attached to a pseudo-terminal (PTY).
// It implements domain.ProcessRunner.
type Runner struct{}

// Run executes the command described by spec using a PTY session and returns its output.
func (r *Runner) Run(ctx context.Context, spec domain.CommandSpec, runDir, prefix string) (domain.ProcessResult, error) {
	if len(spec.Args) == 0 {
		return domain.ProcessResult{}, errors.New("empty command")
	}

	session := NewScriptSession()
	if err := session.Start(ctx, CommandSpec{
		Args: spec.Args,
		Dir:  spec.Dir,
		Env:  spec.Env,
	}); err != nil {
		return domain.ProcessResult{}, err
	}

	if strings.TrimSpace(spec.Stdin) != "" {
		if err := session.Write(spec.Stdin); err != nil {
			_ = session.Close()
			return domain.ProcessResult{}, err
		}
		if !strings.HasSuffix(spec.Stdin, "\n") {
			if err := session.Write("\n"); err != nil {
				_ = session.Close()
				return domain.ProcessResult{}, err
			}
		}
		_ = session.CloseInput()
	}

	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder
	for ev := range session.Events() {
		switch ev.Source {
		case StreamStdout:
			stdoutBuilder.WriteString(ev.Data)
		case StreamStderr:
			stderrBuilder.WriteString(ev.Data)
		}
	}

	waitErr := session.Wait()
	if ctx.Err() != nil {
		return domain.ProcessResult{}, ctx.Err()
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

	return domain.ProcessResult{
		Stdout:   stdoutText,
		Stderr:   stderrText,
		ExitCode: session.ExitCode(),
	}, waitErr
}
