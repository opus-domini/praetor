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

	mu     sync.Mutex
	closed bool
}

const claudeMaxLineBytes = 16 * 1024 * 1024

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
			if isProtocolInvariantFlag(flag) {
				return nil, fmt.Errorf("extra flag %q is not allowed because it overrides protocol settings", flag)
			}
			val := opts.ExtraFlagArgs[k]
			if val == nil {
				args = append(args, flag)
			} else {
				args = append(args, flag, *val)
			}
		}
	}

	if err := validateExtraArgs(opts.ExtraArgs); err != nil {
		return nil, err
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
	t.mu.Lock()
	defer t.mu.Unlock()

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
	t.mu.Lock()
	stdout := t.stdout
	t.mu.Unlock()
	if stdout == nil {
		return errors.New("stdout is closed")
	}
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, err := readLineLimited(reader, claudeMaxLineBytes)
		if len(bytes.TrimSpace(line)) > 0 {
			if handleErr := handle(bytes.TrimSpace(line)); handleErr != nil {
				return handleErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if waitErr := t.waitProcess(); waitErr != nil {
				return fmt.Errorf("process exited: %w", waitErr)
			}
			return nil
		}
		return fmt.Errorf("read stdout: %w", err)
	}
}

func (t *processTransport) endInput() error {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	stdin := t.stdin
	t.stdin = nil
	stdout := t.stdout
	t.stdout = nil
	cmd := t.cmd
	t.mu.Unlock()

	var closeErr error
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = t.waitProcess()
	}
	if stdout != nil {
		if err := stdout.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (t *processTransport) waitProcess() error {
	t.mu.Lock()
	waitCh := t.waitCh
	t.waitCh = nil
	t.mu.Unlock()
	if waitCh == nil {
		return nil
	}
	waitErr, ok := <-waitCh
	if !ok {
		return nil
	}
	return waitErr
}

func readLineLimited(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			if len(line)+len(chunk) > maxBytes {
				return nil, fmt.Errorf("line exceeds maximum size (%d bytes)", maxBytes)
			}
			line = append(line, chunk...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

func isProtocolInvariantFlag(flag string) bool {
	switch flag {
	case "--input-format", "--output-format":
		return true
	default:
		return false
	}
}

func validateExtraArgs(extra []string) error {
	for _, raw := range extra {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		if token == "--" {
			return nil
		}
		if strings.Contains(token, "=") {
			token = strings.SplitN(token, "=", 2)[0]
		}
		if strings.HasPrefix(token, "-") && isProtocolInvariantFlag(token) {
			return fmt.Errorf("extra argument %q is not allowed because it overrides protocol settings", raw)
		}
	}
	return nil
}
