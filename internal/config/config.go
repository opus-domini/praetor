package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/paths"
)

// Config holds resolved configuration values.
// Pointer fields distinguish "not set" from "set to zero/false".
type Config struct {
	Executor      string
	Reviewer      string
	Planner       string
	MaxRetries    *int
	MaxIterations *int
	Runner        string
	Isolation     string
	NoReview      *bool
	NoColor       *bool
	CodexBin      string
	ClaudeBin     string
	GeminiBin     string
	OllamaURL     string
	OllamaModel   string
	Hook          string
	Timeout       string
}

// Path returns the config file path, respecting $PRAETOR_CONFIG.
// Resolution: $PRAETOR_CONFIG > XDG config path > legacy ~/.praetor/config.toml
func Path() string {
	if env := strings.TrimSpace(os.Getenv("PRAETOR_CONFIG")); env != "" {
		return env
	}
	xdgPath, err := paths.DefaultConfigFile()
	if err != nil {
		return ""
	}
	// Read-fallback: if XDG config doesn't exist, check legacy location.
	if _, statErr := os.Stat(xdgPath); statErr != nil {
		if legacy := paths.LegacyConfigFile(); legacy != "" {
			return legacy
		}
	}
	return xdgPath
}

// Load reads the config file and returns resolved values for a project.
// Returns zero Config if the file doesn't exist (no error).
func Load(projectRoot string) (Config, error) {
	path := Path()
	if path == "" {
		return Config{}, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}

	sections, err := parse(string(data))
	if err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}

	global, err := configFromMap("global", sections[""])
	if err != nil {
		return Config{}, fmt.Errorf("load config %s: %w", path, err)
	}

	normalizedProjectRoot, err := normalizeProjectPath(projectRoot)
	if err != nil {
		return Config{}, fmt.Errorf("load config %s: normalize project root: %w", path, err)
	}
	if normalizedProjectRoot == "" {
		return global, nil
	}

	for section, values := range sections {
		if section == "" {
			continue
		}
		normalizedSection, normalizeErr := normalizeProjectPath(section)
		if normalizeErr != nil {
			return Config{}, fmt.Errorf("load config %s: normalize project section %q: %w", path, section, normalizeErr)
		}
		if normalizedSection != normalizedProjectRoot {
			continue
		}

		project, parseErr := configFromMap(section, values)
		if parseErr != nil {
			return Config{}, fmt.Errorf("load config %s: %w", path, parseErr)
		}
		return mergeConfig(global, project), nil
	}
	return global, nil
}

func configFromMap(section string, m map[string]string) (Config, error) {
	var cfg Config
	if v, ok := m["executor"]; ok {
		cfg.Executor = strings.TrimSpace(v)
	}
	if v, ok := m["reviewer"]; ok {
		cfg.Reviewer = strings.TrimSpace(v)
	}
	if v, ok := m["planner"]; ok {
		cfg.Planner = strings.TrimSpace(v)
	}
	if v, ok := m["max-retries"]; ok {
		value, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return Config{}, fmt.Errorf("section %q key %q: invalid integer %q", section, "max-retries", v)
		}
		if value <= 0 {
			return Config{}, fmt.Errorf("section %q key %q: must be greater than zero", section, "max-retries")
		}
		cfg.MaxRetries = &value
	}
	if v, ok := m["max-iterations"]; ok {
		value, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return Config{}, fmt.Errorf("section %q key %q: invalid integer %q", section, "max-iterations", v)
		}
		if value < 0 {
			return Config{}, fmt.Errorf("section %q key %q: cannot be negative", section, "max-iterations")
		}
		cfg.MaxIterations = &value
	}
	if v, ok := m["runner"]; ok {
		cfg.Runner = strings.TrimSpace(v)
	}
	if v, ok := m["isolation"]; ok {
		cfg.Isolation = strings.TrimSpace(v)
	}
	if v, ok := m["no-review"]; ok {
		value, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("section %q key %q: %w", section, "no-review", err)
		}
		cfg.NoReview = &value
	}
	if v, ok := m["no-color"]; ok {
		value, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("section %q key %q: %w", section, "no-color", err)
		}
		cfg.NoColor = &value
	}
	if v, ok := m["codex-bin"]; ok {
		cfg.CodexBin = strings.TrimSpace(v)
	}
	if v, ok := m["claude-bin"]; ok {
		cfg.ClaudeBin = strings.TrimSpace(v)
	}
	if v, ok := m["gemini-bin"]; ok {
		cfg.GeminiBin = strings.TrimSpace(v)
	}
	if v, ok := m["ollama-url"]; ok {
		cfg.OllamaURL = strings.TrimSpace(v)
	}
	if v, ok := m["ollama-model"]; ok {
		cfg.OllamaModel = strings.TrimSpace(v)
	}
	if v, ok := m["hook"]; ok {
		cfg.Hook = strings.TrimSpace(v)
	}
	if v, ok := m["timeout"]; ok {
		timeoutValue := strings.TrimSpace(v)
		d, err := time.ParseDuration(timeoutValue)
		if err != nil {
			return Config{}, fmt.Errorf("section %q key %q: invalid duration %q", section, "timeout", v)
		}
		if d < 0 {
			return Config{}, fmt.Errorf("section %q key %q: cannot be negative", section, "timeout")
		}
		cfg.Timeout = timeoutValue
	}
	return cfg, nil
}

// mergeConfig overlays project values on top of global values.
// Non-zero project values win.
func mergeConfig(global, project Config) Config {
	merged := global
	if project.Executor != "" {
		merged.Executor = project.Executor
	}
	if project.Reviewer != "" {
		merged.Reviewer = project.Reviewer
	}
	if project.Planner != "" {
		merged.Planner = project.Planner
	}
	if project.MaxRetries != nil {
		merged.MaxRetries = project.MaxRetries
	}
	if project.MaxIterations != nil {
		merged.MaxIterations = project.MaxIterations
	}
	if project.Runner != "" {
		merged.Runner = project.Runner
	}
	if project.Isolation != "" {
		merged.Isolation = project.Isolation
	}
	if project.NoReview != nil {
		merged.NoReview = project.NoReview
	}
	if project.NoColor != nil {
		merged.NoColor = project.NoColor
	}
	if project.CodexBin != "" {
		merged.CodexBin = project.CodexBin
	}
	if project.ClaudeBin != "" {
		merged.ClaudeBin = project.ClaudeBin
	}
	if project.GeminiBin != "" {
		merged.GeminiBin = project.GeminiBin
	}
	if project.OllamaURL != "" {
		merged.OllamaURL = project.OllamaURL
	}
	if project.OllamaModel != "" {
		merged.OllamaModel = project.OllamaModel
	}
	if project.Hook != "" {
		merged.Hook = project.Hook
	}
	if project.Timeout != "" {
		merged.Timeout = project.Timeout
	}
	return merged
}

func parseBool(raw string) (bool, error) {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", raw)
	}
}

func normalizeProjectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absPath = filepath.Clean(absPath)
	if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = realPath
	}
	return absPath, nil
}
