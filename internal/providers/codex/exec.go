package codex

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
)

const (
	internalOriginatorEnv = "CODEX_INTERNAL_ORIGINATOR_OVERRIDE"
	goSDKOriginator       = "codex_sdk_go"
)

type execRunArgs struct {
	Input                 string
	BaseURL               string
	APIKey                string
	ThreadID              string
	Images                []string
	Model                 string
	SandboxMode           SandboxMode
	WorkingDirectory      string
	SkipGitRepoCheck      bool
	OutputSchemaFile      string
	ModelReasoningEffort  ModelReasoningEffort
	NetworkAccessEnabled  *bool
	WebSearchMode         WebSearchMode
	WebSearchEnabled      *bool
	ApprovalPolicy        ApprovalMode
	AdditionalDirectories []string
}

type codexExec struct {
	executablePath string
	envOverride    map[string]string
	config         map[string]any
}

func newCodexExec(opts CodexOptions) (*codexExec, error) {
	exePath, err := resolveCodexPath(opts.CodexPathOverride)
	if err != nil {
		return nil, err
	}

	var env map[string]string
	if opts.Env != nil {
		env = make(map[string]string, len(opts.Env))
		for k, v := range opts.Env {
			env[k] = v
		}
	}

	var config map[string]any
	if opts.Config != nil {
		config = make(map[string]any, len(opts.Config))
		for k, v := range opts.Config {
			config[k] = v
		}
	}

	return &codexExec{
		executablePath: exePath,
		envOverride:    env,
		config:         config,
	}, nil
}

func (e *codexExec) run(ctx context.Context, args execRunArgs, onLine func([]byte) error) error {
	commandArgs := []string{"exec", "--experimental-json"}

	configOverrides, err := serializeConfigOverrides(e.config)
	if err != nil {
		return err
	}
	for _, override := range configOverrides {
		commandArgs = append(commandArgs, "--config", override)
	}

	if args.Model != "" {
		commandArgs = append(commandArgs, "--model", args.Model)
	}
	if args.SandboxMode != "" {
		commandArgs = append(commandArgs, "--sandbox", string(args.SandboxMode))
	}
	if args.WorkingDirectory != "" {
		commandArgs = append(commandArgs, "--cd", args.WorkingDirectory)
	}
	for _, dir := range args.AdditionalDirectories {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		commandArgs = append(commandArgs, "--add-dir", dir)
	}
	if args.SkipGitRepoCheck {
		commandArgs = append(commandArgs, "--skip-git-repo-check")
	}
	if args.OutputSchemaFile != "" {
		commandArgs = append(commandArgs, "--output-schema", args.OutputSchemaFile)
	}
	if args.ModelReasoningEffort != "" {
		commandArgs = append(commandArgs, "--config", fmt.Sprintf("model_reasoning_effort=%q", string(args.ModelReasoningEffort)))
	}
	if args.NetworkAccessEnabled != nil {
		commandArgs = append(commandArgs, "--config", fmt.Sprintf("sandbox_workspace_write.network_access=%t", *args.NetworkAccessEnabled))
	}
	if args.WebSearchMode != "" {
		commandArgs = append(commandArgs, "--config", fmt.Sprintf("web_search=%q", string(args.WebSearchMode)))
	} else if args.WebSearchEnabled != nil {
		if *args.WebSearchEnabled {
			commandArgs = append(commandArgs, "--config", `web_search="live"`)
		} else {
			commandArgs = append(commandArgs, "--config", `web_search="disabled"`)
		}
	}
	if args.ApprovalPolicy != "" {
		commandArgs = append(commandArgs, "--config", fmt.Sprintf("approval_policy=%q", string(args.ApprovalPolicy)))
	}
	if args.ThreadID != "" {
		commandArgs = append(commandArgs, "resume", args.ThreadID)
	}
	for _, image := range args.Images {
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		commandArgs = append(commandArgs, "--image", image)
	}

	cmd := exec.CommandContext(ctx, e.executablePath, commandArgs...)
	cmd.Env = buildChildEnv(e.envOverride, args.BaseURL, args.APIKey)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start codex process: %w", err)
	}

	writeErrCh := make(chan error, 1)
	go func() {
		_, err := stdin.Write([]byte(args.Input))
		if closeErr := stdin.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		writeErrCh <- err
		close(writeErrCh)
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := onLine(line); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
	}

	writeErr := <-writeErrCh
	scanErr := scanner.Err()
	waitErr := cmd.Wait()

	if writeErr != nil && !errors.Is(writeErr, os.ErrClosed) {
		return fmt.Errorf("write codex stdin: %w", writeErr)
	}

	if scanErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("read codex stdout: %w", scanErr)
	}

	if waitErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return fmt.Errorf("codex exec failed: %w: %s", waitErr, stderrText)
		}
		return fmt.Errorf("codex exec failed: %w", waitErr)
	}
	return nil
}

func buildChildEnv(override map[string]string, baseURL, apiKey string) []string {
	env := map[string]string{}
	if override != nil {
		for k, v := range override {
			env[k] = v
		}
	} else {
		for _, kv := range os.Environ() {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			env[parts[0]] = parts[1]
		}
	}

	if _, ok := env[internalOriginatorEnv]; !ok {
		env[internalOriginatorEnv] = goSDKOriginator
	}
	if baseURL != "" {
		env["OPENAI_BASE_URL"] = baseURL
	}
	if apiKey != "" {
		env["CODEX_API_KEY"] = apiKey
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func resolveCodexPath(override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		if strings.ContainsRune(override, os.PathSeparator) || strings.HasPrefix(override, ".") {
			abs, err := filepath.Abs(override)
			if err != nil {
				return "", fmt.Errorf("resolve codex override path: %w", err)
			}
			if _, err := os.Stat(abs); err != nil {
				return "", fmt.Errorf("codex override path not found: %w", err)
			}
			return abs, nil
		}
		path, err := exec.LookPath(override)
		if err != nil {
			return "", fmt.Errorf("find codex override command %q: %w", override, err)
		}
		return path, nil
	}

	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}

	if path, ok := findNodeBinCodex(); ok {
		return path, nil
	}
	if path, ok := findVendoredCodex(); ok {
		return path, nil
	}

	return "", errors.New("unable to locate codex executable; set CodexPathOverride or ensure codex is installed")
}

func findNodeBinCodex() (string, bool) {
	candidates := []string{"codex"}
	if runtime.GOOS == "windows" {
		candidates = []string{"codex.cmd", "codex.exe", "codex"}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for _, root := range walkParents(cwd) {
		for _, name := range candidates {
			path := filepath.Join(root, "node_modules", ".bin", name)
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			return path, true
		}
	}
	return "", false
}

func findVendoredCodex() (string, bool) {
	triple, ok := currentTargetTriple()
	if !ok {
		return "", false
	}
	binName := "codex"
	if runtime.GOOS == "windows" {
		binName = "codex.exe"
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for _, root := range walkParents(cwd) {
		path := filepath.Join(root, "node_modules", "@openai", "codex", "vendor", triple, "codex", binName)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		return path, true
	}
	return "", false
}

func currentTargetTriple() (string, bool) {
	switch runtime.GOOS {
	case "linux", "android":
		switch runtime.GOARCH {
		case "amd64":
			return "x86_64-unknown-linux-musl", true
		case "arm64":
			return "aarch64-unknown-linux-musl", true
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "x86_64-apple-darwin", true
		case "arm64":
			return "aarch64-apple-darwin", true
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			return "x86_64-pc-windows-msvc", true
		case "arm64":
			return "aarch64-pc-windows-msvc", true
		}
	}
	return "", false
}

func walkParents(dir string) []string {
	dir = filepath.Clean(dir)
	out := []string{dir}
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		out = append(out, parent)
		dir = parent
	}
}

func serializeConfigOverrides(config map[string]any) ([]string, error) {
	if len(config) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(config))
	if err := flattenConfigOverrides(config, "", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func flattenConfigOverrides(value any, prefix string, out *[]string) error {
	obj, ok := value.(map[string]any)
	if !ok {
		if prefix == "" {
			return errors.New("codex config overrides must be a plain object")
		}
		rendered, err := toTomlValue(value, prefix)
		if err != nil {
			return err
		}
		*out = append(*out, fmt.Sprintf("%s=%s", prefix, rendered))
		return nil
	}

	if prefix != "" && len(obj) == 0 {
		*out = append(*out, fmt.Sprintf("%s={}", prefix))
		return nil
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "" {
			return errors.New("codex config override keys must be non-empty strings")
		}
		child := obj[key]
		if child == nil {
			return fmt.Errorf("codex config override at %s cannot be null", key)
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if nested, ok := asStringMap(child); ok {
			if err := flattenConfigOverrides(nested, path, out); err != nil {
				return err
			}
			continue
		}
		rendered, err := toTomlValue(child, path)
		if err != nil {
			return err
		}
		*out = append(*out, fmt.Sprintf("%s=%s", path, rendered))
	}
	return nil
}

func toTomlValue(value any, path string) (string, error) {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case float64:
		if !isFinite(v) {
			return "", fmt.Errorf("codex config override at %s must be a finite number", path)
		}
		return fmt.Sprintf("%v", v), nil
	case float32:
		f := float64(v)
		if !isFinite(f) {
			return "", fmt.Errorf("codex config override at %s must be a finite number", path)
		}
		return fmt.Sprintf("%v", f), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", v), nil
	case []any:
		parts := make([]string, 0, len(v))
		for idx, item := range v {
			itemValue, err := toTomlValue(item, fmt.Sprintf("%s[%d]", path, idx))
			if err != nil {
				return "", err
			}
			parts = append(parts, itemValue)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]any:
		return toInlineTomlTable(v, path)
	default:
		rv := reflect.ValueOf(value)
		if !rv.IsValid() {
			return "", fmt.Errorf("unsupported codex config override at %s (%T)", path, value)
		}
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			parts := make([]string, 0, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				itemValue, err := toTomlValue(rv.Index(i).Interface(), fmt.Sprintf("%s[%d]", path, i))
				if err != nil {
					return "", err
				}
				parts = append(parts, itemValue)
			}
			return "[" + strings.Join(parts, ", ") + "]", nil
		case reflect.Map:
			if table, ok := asStringMap(value); ok {
				return toInlineTomlTable(table, path)
			}
		}
		return "", fmt.Errorf("unsupported codex config override at %s (%T)", path, value)
	}
}

func toInlineTomlTable(v map[string]any, path string) (string, error) {
	keys := make([]string, 0, len(v))
	for key := range v {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			return "", errors.New("codex config override keys must be non-empty strings")
		}
		child := v[key]
		if child == nil {
			return "", fmt.Errorf("codex config override at %s.%s cannot be null", path, key)
		}
		rendered, err := toTomlValue(child, path+"."+key)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s = %s", formatTomlKey(key), rendered))
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}

func asStringMap(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, false
	}

	out := make(map[string]any, rv.Len())
	for _, key := range rv.MapKeys() {
		out[key.String()] = rv.MapIndex(key).Interface()
	}
	return out, true
}

func formatTomlKey(key string) string {
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return fmt.Sprintf("%q", key)
	}
	return key
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
