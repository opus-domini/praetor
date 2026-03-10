package tmux

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

	"github.com/opus-domini/praetor/internal/domain"
)

// Runner executes commands inside tmux windows.
// It implements domain.ProcessRunner and domain.SessionManager.
type Runner struct {
	sessionName  string
	formatterBin string // path to "praetor" binary for live formatting; empty disables

	mu             sync.Mutex
	prepared       bool
	createdSession bool
}

// NewRunner creates a tmux runner targeting the given session name.
// formatterBin is the path to the praetor binary used for live formatting
// of JSONL output in tmux panes. Pass "" to disable formatting.
func NewRunner(sessionName, formatterBin string) *Runner {
	return &Runner{
		sessionName:  strings.TrimSpace(sessionName),
		formatterBin: strings.TrimSpace(formatterBin),
	}
}

// SessionName returns the tmux session target used by this runner.
func (r *Runner) SessionName() string {
	return r.sessionName
}

// EnsureSession validates tmux availability and creates the target session if needed.
func (r *Runner) EnsureSession() error {
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
func (r *Runner) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.createdSession {
		return
	}
	_ = exec.Command("tmux", "kill-session", "-t", r.sessionName).Run()
	r.prepared = false
	r.createdSession = false
}

// Run executes a domain.CommandSpec in a dedicated tmux window.
func (r *Runner) Run(ctx context.Context, spec domain.CommandSpec, runDir, prefix string) (domain.ProcessResult, error) {
	if err := r.EnsureSession(); err != nil {
		return domain.ProcessResult{}, err
	}

	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		return domain.ProcessResult{}, errors.New("run directory is required for tmux runner")
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return domain.ProcessResult{}, fmt.Errorf("create run directory: %w", err)
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
			return domain.ProcessResult{}, fmt.Errorf("write stdin file: %w", err)
		}
	}

	channel := fmt.Sprintf("praetor-%d-%d", os.Getpid(), time.Now().UnixNano())
	script := BuildWrapperScript(spec, stdinFile, stdoutFile, stderrFile, exitFile, channel, r.formatterBin)
	if err := os.WriteFile(wrapperFile, []byte(script), 0o755); err != nil {
		return domain.ProcessResult{}, fmt.Errorf("write wrapper script: %w", err)
	}

	windowName := WindowName(spec.WindowHint, prefix)
	createWindow := exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}",
		"-t", r.sessionName+":", "-n", windowName,
		"bash", wrapperFile)
	windowOut, err := createWindow.CombinedOutput()
	if err != nil {
		return domain.ProcessResult{}, fmt.Errorf("create tmux window: %w: %s", err, strings.TrimSpace(string(windowOut)))
	}
	windowID := strings.TrimSpace(string(windowOut))
	defer KillWindow(windowID)

	// Focus this window so users attaching to the session see live output.
	_ = exec.Command("tmux", "select-window", "-t", windowID).Run()

	waitCmd := exec.CommandContext(ctx, "tmux", "wait-for", channel)
	if output, err := waitCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			KillWindow(windowID)
			return domain.ProcessResult{}, ctx.Err()
		}
		return domain.ProcessResult{}, fmt.Errorf("wait for tmux channel: %w: %s", err, strings.TrimSpace(string(output)))
	}

	stdout, _ := os.ReadFile(stdoutFile)
	stderr, _ := os.ReadFile(stderrFile)

	exitBytes, readExitErr := os.ReadFile(exitFile)
	if readExitErr != nil {
		return domain.ProcessResult{
			Stdout: strings.TrimSpace(string(stdout)),
			Stderr: strings.TrimSpace(string(stderr)),
		}, fmt.Errorf("agent exit status file not found: %w: %s", readExitErr, TailText(string(stderr), 20))
	}

	exitCode, parseErr := strconv.Atoi(strings.TrimSpace(string(exitBytes)))
	if parseErr != nil {
		return domain.ProcessResult{
			Stdout: strings.TrimSpace(string(stdout)),
			Stderr: strings.TrimSpace(string(stderr)),
		}, fmt.Errorf("invalid agent exit code: %w: %s", parseErr, TailText(string(stderr), 20))
	}

	return domain.ProcessResult{
		Stdout:   strings.TrimSpace(string(stdout)),
		Stderr:   strings.TrimSpace(string(stderr)),
		ExitCode: exitCode,
	}, nil
}

// KillWindow kills a tmux window by ID.
func KillWindow(windowID string) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return
	}
	_ = exec.Command("tmux", "kill-window", "-t", windowID).Run()
}

// WindowName builds a sanitized tmux window name from a task label and role.
func WindowName(taskLabel, role string) string {
	taskLabel = strings.TrimSpace(taskLabel)
	role = strings.TrimSpace(role)
	if role == "" {
		role = "agent"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	var name string
	if taskLabel != "" {
		name = replacer.Replace(taskLabel) + "-" + replacer.Replace(role)
	} else {
		name = "praetor-" + replacer.Replace(role)
	}
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

// BuildWrapperScript generates a bash wrapper script for tmux execution.
// formatterBin is the path to the praetor binary; when non-empty the pane
// shows formatted output while raw JSONL is still captured to stdoutFile.
func BuildWrapperScript(spec domain.CommandSpec, stdinFile, stdoutFile, stderrFile, exitFile, channel, formatterBin string) string {
	// Build the command line from spec.Args with proper quoting.
	var cmdParts []string
	for _, arg := range spec.Args {
		cmdParts = append(cmdParts, ShellQuote(arg))
	}
	cmdLine := strings.Join(cmdParts, " ")

	// Handle stdin redirection if a stdin file was provided.
	stdinClause := ""
	if stdinFile != "" {
		stdinClause = fmt.Sprintf(" < %s", ShellQuote(stdinFile))
	}

	// Handle environment variables.
	var envLines string
	for _, env := range spec.Env {
		envLines += fmt.Sprintf("export %s\n", ShellQuote(env))
	}

	dir := strings.TrimSpace(spec.Dir)
	if dir == "" {
		dir = "."
	}

	// Build a short banner showing the binary and key flags (not the prompt).
	banner := spec.Args[0]
	for _, arg := range spec.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			banner += " " + arg
		} else if len(arg) < 40 {
			banner += " " + arg
		}
	}
	if len(banner) > 120 {
		banner = banner[:120] + "..."
	}

	// When a formatter binary is available, pipe pane output through it
	// so the user sees human-readable text instead of raw JSONL.
	// Raw output is still captured to stdoutFile via tee.
	fmtPipe := ""
	if strings.TrimSpace(formatterBin) != "" {
		fmtPipe = " | " + ShellQuote(formatterBin) + " fmt 2>/dev/null"
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
_exit=1
trap 'printf "%%s" "$_exit" > %s; tmux wait-for -S %s' EXIT
set -euo pipefail

# Strip nesting-detection variables so spawned agents start normally.
unset %s

printf '\033[1;34m▸ %%s\033[0m\n' %s
printf '\033[2m  dir: %%s\033[0m\n\n' %s

%scd %s

%s%s \
  2> >(tee %s >&2) | tee %s%s
_exit="${PIPESTATUS[0]}"
`, ShellQuote(exitFile), ShellQuote(channel),
		strings.Join(domain.AgentNestingEnvVars, " "),
		ShellQuote(banner), ShellQuote(dir),
		envLines,
		ShellQuote(dir),
		cmdLine, stdinClause,
		ShellQuote(stderrFile), ShellQuote(stdoutFile), fmtPipe)
}

// ShellQuote wraps a value in single quotes suitable for bash.
func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// TailText returns the last maxLines lines of value, joined with " | ".
func TailText(value string, maxLines int) string {
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
