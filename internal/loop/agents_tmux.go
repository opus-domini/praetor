package loop

import (
	"context"
	"encoding/json"
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

// TMUXAgentRuntime executes every agent step inside tmux windows.
type TMUXAgentRuntime struct {
	sessionName string

	mu             sync.Mutex
	prepared       bool
	createdSession bool
}

// NewTMUXAgentRuntime creates a tmux-backed runtime.
func NewTMUXAgentRuntime(sessionName string) *TMUXAgentRuntime {
	return &TMUXAgentRuntime{sessionName: strings.TrimSpace(sessionName)}
}

// SessionName returns the tmux session target used by this runtime.
func (r *TMUXAgentRuntime) SessionName() string {
	return r.sessionName
}

// EnsureSession validates tmux availability and creates the target session if needed.
func (r *TMUXAgentRuntime) EnsureSession() error {
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

// Cleanup kills the tmux session if this runtime created it.
func (r *TMUXAgentRuntime) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.createdSession {
		return
	}
	_ = exec.Command("tmux", "kill-session", "-t", r.sessionName).Run()
	r.prepared = false
	r.createdSession = false
}

// codexJSONOutput is the shape of Codex --json output.
type codexJSONOutput struct {
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// Run executes one agent invocation in a dedicated tmux window.
func (r *TMUXAgentRuntime) Run(ctx context.Context, req AgentRequest) (AgentResult, error) {
	start := time.Now()
	if err := r.EnsureSession(); err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, err
	}

	runDir := strings.TrimSpace(req.RunDir)
	if runDir == "" {
		return AgentResult{DurationS: time.Since(start).Seconds()}, errors.New("run directory is required for tmux runtime")
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, fmt.Errorf("create run directory: %w", err)
	}

	prefix := strings.TrimSpace(req.OutputPrefix)
	if prefix == "" {
		prefix = "agent"
	}

	promptFile := filepath.Join(runDir, prefix+".prompt")
	systemFile := filepath.Join(runDir, prefix+".system-prompt")
	stdoutFile := filepath.Join(runDir, prefix+".stdout")
	stderrFile := filepath.Join(runDir, prefix+".stderr")
	exitFile := filepath.Join(runDir, prefix+".exit")
	wrapperFile := filepath.Join(runDir, prefix+".run.sh")

	if err := os.WriteFile(promptFile, []byte(strings.TrimSpace(req.Prompt)), 0o644); err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, fmt.Errorf("write prompt file: %w", err)
	}
	if err := os.WriteFile(systemFile, []byte(strings.TrimSpace(req.SystemPrompt)), 0o644); err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, fmt.Errorf("write system prompt file: %w", err)
	}

	channel := fmt.Sprintf("praetor-%d-%d", os.Getpid(), time.Now().UnixNano())
	windowName := tmuxWindowName(req)
	script := buildWrapperScript(req, promptFile, systemFile, stdoutFile, stderrFile, exitFile, channel)

	if err := os.WriteFile(wrapperFile, []byte(script), 0o755); err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, fmt.Errorf("write wrapper script: %w", err)
	}

	createWindow := exec.Command("tmux", "new-window", "-d", "-t", r.sessionName+":", "-n", windowName, "bash", wrapperFile)
	if output, err := createWindow.CombinedOutput(); err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, fmt.Errorf("create tmux window: %w: %s", err, strings.TrimSpace(string(output)))
	}

	waitCmd := exec.CommandContext(ctx, "tmux", "wait-for", channel)
	if output, err := waitCmd.CombinedOutput(); err != nil {
		duration := time.Since(start)
		if ctx.Err() != nil {
			return AgentResult{DurationS: duration.Seconds()}, ctx.Err()
		}
		return AgentResult{DurationS: duration.Seconds()}, fmt.Errorf("wait for tmux channel: %w: %s", err, strings.TrimSpace(string(output)))
	}

	outputBytes, _ := os.ReadFile(stdoutFile)
	outputText := strings.TrimSpace(string(outputBytes))
	duration := time.Since(start)

	exitBytes, readExitErr := os.ReadFile(exitFile)
	if readExitErr != nil {
		stderrBytes, _ := os.ReadFile(stderrFile)
		return AgentResult{Output: outputText, DurationS: duration.Seconds()}, fmt.Errorf("agent exit status file not found: %w: %s", readExitErr, tailText(string(stderrBytes), 20))
	}

	exitCode, parseErr := strconv.Atoi(strings.TrimSpace(string(exitBytes)))
	if parseErr != nil {
		stderrBytes, _ := os.ReadFile(stderrFile)
		return AgentResult{Output: outputText, DurationS: duration.Seconds()}, fmt.Errorf("invalid agent exit code: %w: %s", parseErr, tailText(string(stderrBytes), 20))
	}
	if exitCode != 0 {
		stderrBytes, _ := os.ReadFile(stderrFile)
		return AgentResult{Output: outputText, DurationS: duration.Seconds()}, fmt.Errorf("agent process failed with exit code %d: %s", exitCode, tailText(string(stderrBytes), 20))
	}

	// Post-process Codex JSON output: extract .result and .total_cost_usd
	var costUSD float64
	if strings.HasPrefix(outputText, "{") {
		var parsed codexJSONOutput
		if err := json.Unmarshal([]byte(outputText), &parsed); err == nil {
			// Save raw JSON
			rawFile := filepath.Join(runDir, prefix+".raw.json")
			_ = os.WriteFile(rawFile, []byte(outputText), 0o644)
			costUSD = parsed.TotalCostUSD
			if parsed.Result != "" {
				outputText = parsed.Result
			}
		}
	}

	return AgentResult{
		Output:    outputText,
		CostUSD:   costUSD,
		DurationS: duration.Seconds(),
	}, nil
}

func tmuxWindowName(req AgentRequest) string {
	label := strings.TrimSpace(req.TaskLabel)
	if label == "" {
		label = "task"
	}
	prefix := strings.TrimSpace(req.OutputPrefix)
	if prefix == "" {
		prefix = "agent"
	}

	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	name := "praetor-" + replacer.Replace(label) + "-" + replacer.Replace(prefix)
	if len(name) > 48 {
		name = name[:48]
	}
	if name == "" {
		return "praetor-agent"
	}
	return name
}

func buildWrapperScript(req AgentRequest, promptFile, systemFile, stdoutFile, stderrFile, exitFile, channel string) string {
	agent := normalizeAgent(req.Agent)
	workdir := strings.TrimSpace(req.Workdir)
	if workdir == "" {
		workdir = "."
	}
	codexBin := strings.TrimSpace(req.CodexBin)
	if codexBin == "" {
		codexBin = "codex"
	}
	claudeBin := strings.TrimSpace(req.ClaudeBin)
	if claudeBin == "" {
		claudeBin = "claude"
	}
	model := strings.TrimSpace(req.Model)

	var commandBlock string
	if agent == AgentCodex {
		commandBlock = buildCodexCommandBlock(codexBin, model)
	} else {
		commandBlock = buildClaudeCommandBlock(claudeBin, model, req.Verbose)
	}

	return strings.TrimSpace(fmt.Sprintf(`#!/usr/bin/env bash
_exit=1
trap 'printf "%%s" "$_exit" > %s; tmux wait-for -S %s' EXIT
set -uo pipefail

PROMPT_FILE=%s
SYSTEM_FILE=%s
STDOUT_FILE=%s
STDERR_FILE=%s
WORKDIR=%s

cd "$WORKDIR"

%s
`, shellQuote(exitFile), shellQuote(channel), shellQuote(promptFile), shellQuote(systemFile), shellQuote(stdoutFile), shellQuote(stderrFile), shellQuote(workdir), commandBlock)) + "\n"
}

func buildCodexCommandBlock(codexBin, model string) string {
	modelClause := ""
	if model != "" {
		modelClause = fmt.Sprintf("  --model %s \\\n", shellQuote(model))
	}

	return fmt.Sprintf(`FULL_PROMPT="$(cat "$PROMPT_FILE")"
if [[ -s "$SYSTEM_FILE" ]]; then
  FULL_PROMPT="$(cat "$SYSTEM_FILE")"$'\n\n'"$FULL_PROMPT"
fi

%s exec --json \
%s  "$FULL_PROMPT" \
  2> >(tee "$STDERR_FILE" >&2) | tee "$STDOUT_FILE"
_exit="${PIPESTATUS[0]}"`, shellQuote(codexBin), modelClause)
}

func buildClaudeCommandBlock(claudeBin, model string, verbose bool) string {
	modelClause := ""
	if model != "" {
		modelClause = fmt.Sprintf("  --model %s \\\n", shellQuote(model))
	}
	verboseClause := ""
	if verbose {
		verboseClause = "  --verbose \\\n"
	}

	return fmt.Sprintf(`SYSTEM_ARGS=()
if [[ -s "$SYSTEM_FILE" ]]; then
  SYSTEM_ARGS=(--append-system-prompt "$(cat \"$SYSTEM_FILE\")")
fi

%s -p \
  --dangerously-skip-permissions \
  --no-session-persistence \
%s%s  "${SYSTEM_ARGS[@]}" \
  < "$PROMPT_FILE" \
  2> >(tee "$STDERR_FILE" >&2) | tee "$STDOUT_FILE"
_exit="${PIPESTATUS[0]}"`, shellQuote(claudeBin), modelClause, verboseClause)
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
