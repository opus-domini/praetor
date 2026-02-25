package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tmuxRunner executes commands inside tmux windows.
// It implements ProcessRunner and SessionManager.
type tmuxRunner struct {
	sessionName string

	mu             sync.Mutex
	prepared       bool
	createdSession bool
}

func newTmuxRunner(sessionName string) *tmuxRunner {
	return &tmuxRunner{sessionName: strings.TrimSpace(sessionName)}
}

// SessionName returns the tmux session target used by this runner.
func (r *tmuxRunner) SessionName() string {
	return r.sessionName
}

// EnsureSession validates tmux availability and creates the target session if needed.
func (r *tmuxRunner) EnsureSession() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.prepared {
		return nil
	}
	if strings.TrimSpace(r.sessionName) == "" {
		return errors.New("tmux session name is required")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return errors.New("tmux command not found in PATH")
	}

	hasSession := exec.Command("tmux", "has-session", "-t", r.sessionName)
	if err := hasSession.Run(); err != nil {
		createSession := exec.Command("tmux", "new-session", "-d", "-s", r.sessionName)
		if output, createErr := createSession.CombinedOutput(); createErr != nil {
			return fmt.Errorf("create tmux session %s: %w: %s", r.sessionName, createErr, strings.TrimSpace(string(output)))
		}
		r.createdSession = true
	}

	r.prepared = true
	return nil
}

// Cleanup kills the tmux session if this runner created it.
func (r *tmuxRunner) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.createdSession {
		return
	}
	_ = exec.Command("tmux", "kill-session", "-t", r.sessionName).Run()
	r.prepared = false
	r.createdSession = false
}

// Run executes a CommandSpec in a dedicated tmux window.
func (r *tmuxRunner) Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error) {
	if err := r.EnsureSession(); err != nil {
		return ProcessResult{}, err
	}

	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		return ProcessResult{}, errors.New("run directory is required for tmux runner")
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return ProcessResult{}, fmt.Errorf("create run directory: %w", err)
	}

	if prefix == "" {
		prefix = "agent"
	}

	stdoutFile := filepath.Join(runDir, prefix+".stdout")
	stderrFile := filepath.Join(runDir, prefix+".stderr")
	exitFile := filepath.Join(runDir, prefix+".exit")
	wrapperFile := filepath.Join(runDir, prefix+".run.sh")

	// Write stdin content to a file if provided.
	var stdinFile string
	if spec.Stdin != "" {
		stdinFile = filepath.Join(runDir, prefix+".stdin")
		if err := os.WriteFile(stdinFile, []byte(spec.Stdin), 0o644); err != nil {
			return ProcessResult{}, fmt.Errorf("write stdin file: %w", err)
		}
	}

	channel := fmt.Sprintf("praetor-%d-%d", os.Getpid(), time.Now().UnixNano())
	script := buildTmuxWrapperScript(spec, stdinFile, stdoutFile, stderrFile, exitFile, channel)
	if err := os.WriteFile(wrapperFile, []byte(script), 0o755); err != nil {
		return ProcessResult{}, fmt.Errorf("write wrapper script: %w", err)
	}

	windowName := tmuxWindowName(prefix)
	createWindow := exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}", "-t", r.sessionName+":", "-n", windowName, "bash", wrapperFile)
	windowOut, err := createWindow.CombinedOutput()
	if err != nil {
		return ProcessResult{}, fmt.Errorf("create tmux window: %w: %s", err, strings.TrimSpace(string(windowOut)))
	}
	windowID := strings.TrimSpace(string(windowOut))

	waitCmd := exec.CommandContext(ctx, "tmux", "wait-for", channel)
	if output, err := waitCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			killTMUXWindow(windowID)
			return ProcessResult{}, ctx.Err()
		}
		return ProcessResult{}, fmt.Errorf("wait for tmux channel: %w: %s", err, strings.TrimSpace(string(output)))
	}

	stdout, _ := os.ReadFile(stdoutFile)
	stderr, _ := os.ReadFile(stderrFile)

	exitBytes, readExitErr := os.ReadFile(exitFile)
	if readExitErr != nil {
		return ProcessResult{
			Stdout: strings.TrimSpace(string(stdout)),
			Stderr: strings.TrimSpace(string(stderr)),
		}, fmt.Errorf("agent exit status file not found: %w: %s", readExitErr, tailText(string(stderr), 20))
	}

	exitCode, parseErr := strconv.Atoi(strings.TrimSpace(string(exitBytes)))
	if parseErr != nil {
		return ProcessResult{
			Stdout: strings.TrimSpace(string(stdout)),
			Stderr: strings.TrimSpace(string(stderr)),
		}, fmt.Errorf("invalid agent exit code: %w: %s", parseErr, tailText(string(stderr), 20))
	}

	return ProcessResult{
		Stdout:   strings.TrimSpace(string(stdout)),
		Stderr:   strings.TrimSpace(string(stderr)),
		ExitCode: exitCode,
	}, nil
}

func killTMUXWindow(windowID string) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return
	}
	_ = exec.Command("tmux", "kill-window", "-t", windowID).Run()
}

func tmuxWindowName(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "agent"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	name := "praetor-" + replacer.Replace(prefix)
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

func buildTmuxWrapperScript(spec CommandSpec, stdinFile, stdoutFile, stderrFile, exitFile, channel string) string {
	// Build the command line from spec.Args with proper quoting.
	var cmdParts []string
	for _, arg := range spec.Args {
		cmdParts = append(cmdParts, shellQuote(arg))
	}
	cmdLine := strings.Join(cmdParts, " ")

	// Handle stdin redirection if a stdin file was provided.
	stdinClause := ""
	if stdinFile != "" {
		stdinClause = fmt.Sprintf(" < %s", shellQuote(stdinFile))
	}

	// Handle environment variables.
	var envLines string
	for _, env := range spec.Env {
		envLines += fmt.Sprintf("export %s\n", shellQuote(env))
	}

	dir := strings.TrimSpace(spec.Dir)
	if dir == "" {
		dir = "."
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
_exit=1
trap 'printf "%%s" "$_exit" > %s; tmux wait-for -S %s' EXIT
set -euo pipefail

%scd %s

%s%s \
  2> >(tee %s >&2) | tee %s
_exit="${PIPESTATUS[0]}"
`, shellQuote(exitFile), shellQuote(channel),
		envLines,
		shellQuote(dir),
		cmdLine, stdinClause,
		shellQuote(stderrFile), shellQuote(stdoutFile))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func tailText(value string, maxLines int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "no stderr output"
	}
	lines := strings.Split(value, "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, " | ")
	}
	return strings.Join(lines[len(lines)-maxLines:], " | ")
}
