package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type processTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	waitCh chan error

	writeMu sync.Mutex
	closeMu sync.Mutex
	closed  bool
}

func newProcessTransport(ctx context.Context, opts Options) (*processTransport, error) {
	command := strings.TrimSpace(opts.PathToClaudeCodeExecutable)
	if command == "" {
		command = strings.TrimSpace(opts.Command)
	}
	if command == "" {
		command = "claude"
	}

	args, err := buildCLIArgs(opts)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, command, args...)
	if opts.CWD != "" {
		cmd.Dir = opts.CWD
	}
	cmd.Env = buildProcessEnv(opts)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdout pipe: %w", err)
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
		close(waitCh)
	}()

	return &processTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		waitCh: waitCh,
	}, nil
}

func buildCLIArgs(opts Options) ([]string, error) {
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
		"--input-format", "stream-json",
	}

	if opts.Thinking != nil {
		switch opts.Thinking.Type {
		case ThinkingAdaptive:
			args = append(args, "--thinking", "adaptive")
		case ThinkingDisabled:
			args = append(args, "--thinking", "disabled")
		case ThinkingEnabled:
			if opts.Thinking.BudgetTokens == nil {
				args = append(args, "--thinking", "adaptive")
			} else {
				args = append(args, "--max-thinking-tokens", strconv.Itoa(*opts.Thinking.BudgetTokens))
			}
		}
	} else if opts.MaxThinkingTokens != nil {
		if *opts.MaxThinkingTokens == 0 {
			args = append(args, "--thinking", "disabled")
		} else {
			args = append(args, "--max-thinking-tokens", strconv.Itoa(*opts.MaxThinkingTokens))
		}
	}

	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	if opts.MaxTurns != nil {
		args = append(args, "--max-turns", strconv.Itoa(*opts.MaxTurns))
	}
	if opts.MaxBudgetUSD != nil {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(*opts.MaxBudgetUSD, 'f', -1, 64))
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Agent != "" {
		args = append(args, "--agent", opts.Agent)
	}
	if len(opts.Betas) > 0 {
		args = append(args, "--betas", strings.Join(opts.Betas, ","))
	}
	jsonSchema := opts.JSONSchema
	if opts.OutputFormat != nil && len(opts.OutputFormat.Schema) > 0 {
		jsonSchema = opts.OutputFormat.Schema
	}
	if len(jsonSchema) > 0 {
		args = append(args, "--json-schema", string(jsonSchema))
	}
	if opts.DebugFile != "" {
		args = append(args, "--debug-file", opts.DebugFile)
	} else if opts.Debug {
		args = append(args, "--debug")
	}
	if opts.CanUseTool != nil {
		if opts.PermissionPromptToolName != "" {
			return nil, errors.New("canUseTool cannot be combined with PermissionPromptToolName")
		}
		args = append(args, "--permission-prompt-tool", "stdio")
	} else if opts.PermissionPromptToolName != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptToolName)
	}
	if opts.ContinueConversation {
		args = append(args, "--continue")
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}
	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}
	if len(opts.Tools) > 0 {
		args = append(args, "--tools", strings.Join(opts.Tools, ","))
	}
	if len(opts.MCPServers) > 0 {
		payload := map[string]any{"mcpServers": opts.MCPServers}
		j, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal --mcp-config: %w", err)
		}
		args = append(args, "--mcp-config", string(j))
	}
	if len(opts.SettingSources) > 0 {
		ss := make([]string, 0, len(opts.SettingSources))
		for _, s := range opts.SettingSources {
			ss = append(ss, string(s))
		}
		args = append(args, "--setting-sources", strings.Join(ss, ","))
	}
	if opts.StrictMCPConfig {
		args = append(args, "--strict-mcp-config")
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", string(opts.PermissionMode))
	}
	if opts.AllowDangerouslySkipPermissions {
		args = append(args, "--allow-dangerously-skip-permissions")
	}
	if opts.FallbackModel != "" {
		if opts.Model != "" && opts.FallbackModel == opts.Model {
			return nil, errors.New("fallback model cannot be the same as primary model")
		}
		args = append(args, "--fallback-model", opts.FallbackModel)
	}
	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}
	for _, p := range opts.Plugins {
		if strings.EqualFold(p.Type, "local") && strings.TrimSpace(p.Path) != "" {
			args = append(args, "--plugin-dir", p.Path)
		}
	}
	if opts.ForkSession {
		args = append(args, "--fork-session")
	}
	if opts.ResumeSessionAt != "" {
		args = append(args, "--resume-session-at", opts.ResumeSessionAt)
	}
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	if opts.PersistSession != nil && !*opts.PersistSession {
		args = append(args, "--no-session-persistence")
	}
	for _, dir := range opts.AdditionalDirectories {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		args = append(args, "--add-dir", dir)
	}
	if opts.Sandbox != nil {
		settingsObj := map[string]any{"sandbox": opts.Sandbox}
		j, err := json.Marshal(settingsObj)
		if err != nil {
			return nil, fmt.Errorf("marshal --settings: %w", err)
		}
		args = append(args, "--settings", string(j))
	}
	if len(opts.ExtraFlagArgs) > 0 {
		keys := make([]string, 0, len(opts.ExtraFlagArgs))
		for k := range opts.ExtraFlagArgs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flag := "--" + strings.TrimLeft(k, "-")
			val := opts.ExtraFlagArgs[k]
			if val == nil {
				args = append(args, flag)
			} else {
				args = append(args, flag, *val)
			}
		}
	}

	args = append(args, opts.ExtraArgs...)
	return args, nil
}

func buildProcessEnv(opts Options) []string {
	var env []string
	if opts.Env != nil {
		env = append(env, opts.Env...)
	} else {
		env = append(env, os.Environ()...)
	}

	overlay := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			overlay[parts[0]] = parts[1]
		}
	}
	for k, v := range opts.EnvMap {
		overlay[k] = v
	}
	if _, ok := overlay["CLAUDE_CODE_ENTRYPOINT"]; !ok {
		overlay["CLAUDE_CODE_ENTRYPOINT"] = "sdk-go"
	}
	if opts.EnableFileCheckpointing {
		overlay["CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING"] = "true"
	}

	out := make([]string, 0, len(overlay))
	for k, v := range overlay {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func (t *processTransport) writeJSONLine(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal outbound json: %w", err)
	}
	return t.writeLine(line)
}

func (t *processTransport) writeLine(line []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if t.closed {
		return errors.New("transport is closed")
	}
	if t.stdin == nil {
		return errors.New("stdin is closed")
	}
	if len(line) == 0 {
		return nil
	}
	if !bytes.HasSuffix(line, []byte("\n")) {
		line = append(line, '\n')
	}
	if _, err := t.stdin.Write(line); err != nil {
		return fmt.Errorf("write stdin: %w", err)
	}
	return nil
}

func (t *processTransport) readLines(handle func([]byte) error) error {
	reader := bufio.NewReader(t.stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if handleErr := handle(bytes.TrimSpace(line)); handleErr != nil {
				return handleErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if t.waitCh != nil {
				waitErr := <-t.waitCh
				if waitErr != nil {
					return fmt.Errorf("process exited: %w", waitErr)
				}
			}
			return nil
		}
		return fmt.Errorf("read stdout: %w", err)
	}
}

func (t *processTransport) endInput() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()
	if t.closed || t.stdin == nil {
		return nil
	}
	if err := t.stdin.Close(); err != nil {
		return fmt.Errorf("close stdin: %w", err)
	}
	t.stdin = nil
	return nil
}

func (t *processTransport) close() error {
	t.closeMu.Lock()
	defer t.closeMu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	var closeErr error
	if t.stdin != nil {
		_ = t.stdin.Close()
		t.stdin = nil
	}

	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		if t.waitCh != nil {
			<-t.waitCh
		}
	}

	if t.stdout != nil {
		if err := t.stdout.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		t.stdout = nil
	}

	return closeErr
}
