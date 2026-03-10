package runner

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
	ptyruntime "github.com/opus-domini/praetor/internal/runtime/pty"
)

// CommandSpec describes one command invocation.
type CommandSpec struct {
	Args         []string
	Env          []string
	Dir          string
	Stdin        string
	UsePTY       bool
	RunDir       string
	OutputPrefix string
	WindowHint   string
}

// CommandResult holds raw process output.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Strategy string
}

// CommandRunner executes command specs.
type CommandRunner interface {
	Run(ctx context.Context, spec CommandSpec) (CommandResult, error)
}

// ExecCommandRunnerOptions tunes PTY behavior for command execution.
type ExecCommandRunnerOptions struct {
	ForcePTY   bool
	DisablePTY bool
}

// execCommandRunner executes commands directly, with optional PTY fallback.
type execCommandRunner struct {
	options ExecCommandRunnerOptions
}

func NewExecCommandRunner() CommandRunner {
	return &execCommandRunner{}
}

func NewExecCommandRunnerWithOptions(options ExecCommandRunnerOptions) CommandRunner {
	return &execCommandRunner{options: options}
}

func (r *execCommandRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	if len(spec.Args) == 0 {
		return CommandResult{}, errors.New("empty command")
	}

	usePTY := spec.UsePTY
	if r.options.ForcePTY {
		usePTY = true
	}
	if r.options.DisablePTY {
		usePTY = false
	}
	if usePTY {
		return runWithPTY(ctx, spec)
	}
	result, err := runWithoutPTY(ctx, spec)
	if ctx.Err() != nil {
		return CommandResult{}, ctx.Err()
	}
	if err == nil && result.ExitCode == 0 {
		return result, nil
	}
	if r.options.DisablePTY {
		return result, err
	}
	if shouldFallbackToPTY(result, err) {
		fallbackSpec := spec
		fallbackSpec.UsePTY = true
		ptyResult, ptyErr := runWithPTY(ctx, fallbackSpec)
		if ptyErr == nil || ptyResult.ExitCode == 0 {
			return ptyResult, ptyErr
		}
	}
	return result, err
}

func runWithoutPTY(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	if strings.TrimSpace(spec.Dir) != "" {
		cmd.Dir = spec.Dir
	}
	cmd.Env = domain.CleanAgentEnv(spec.Env)
	if strings.TrimSpace(spec.Stdin) != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}

	var stdoutBuilder strings.Builder
	var stderrBuilder strings.Builder
	cmd.Stdout = &stdoutBuilder
	cmd.Stderr = &stderrBuilder

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil
		}
	}
	if ctx.Err() != nil {
		return CommandResult{}, ctx.Err()
	}

	return CommandResult{
		Stdout:   strings.TrimSpace(stdoutBuilder.String()),
		Stderr:   strings.TrimSpace(stderrBuilder.String()),
		ExitCode: exitCode,
		Strategy: "process",
	}, err
}

func shouldFallbackToPTY(result CommandResult, err error) bool {
	combined := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(result.Stderr),
		strings.TrimSpace(result.Stdout),
	}, "\n")))
	if err != nil {
		if combined == "" {
			combined = strings.ToLower(strings.TrimSpace(err.Error()))
		} else {
			combined = combined + "\n" + strings.ToLower(strings.TrimSpace(err.Error()))
		}
	}
	if combined == "" {
		return false
	}
	patterns := []string{
		"not a tty",
		"requires a tty",
		"stdin is not a tty",
		"terminal is required",
	}
	for _, pattern := range patterns {
		if strings.Contains(combined, pattern) {
			return true
		}
	}
	return false
}

func runWithPTY(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	session := ptyruntime.NewScriptSession()
	if err := session.Start(ctx, ptyruntime.CommandSpec{
		Args: spec.Args,
		Dir:  spec.Dir,
		Env:  spec.Env,
	}); err != nil {
		return CommandResult{}, err
	}

	if strings.TrimSpace(spec.Stdin) != "" {
		if err := session.Write(spec.Stdin); err != nil {
			_ = session.Close()
			return CommandResult{}, err
		}
		if !strings.HasSuffix(spec.Stdin, "\n") {
			if err := session.Write("\n"); err != nil {
				_ = session.Close()
				return CommandResult{}, err
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
	exitCode := session.ExitCode()
	if ctx.Err() != nil {
		return CommandResult{}, ctx.Err()
	}

	return CommandResult{
		Stdout:   strings.TrimSpace(stdoutBuilder.String()),
		Stderr:   strings.TrimSpace(stderrBuilder.String()),
		ExitCode: exitCode,
		Strategy: "pty",
	}, waitErr
}
