package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config holds resolved configuration values.
// Pointer fields distinguish "not set" from "set to zero/false".
type Config struct {
	Executor      string
	Reviewer      string
	MaxRetries    *int
	MaxIterations *int
	Isolation     string
	NoReview      *bool
	NoColor       *bool
	CodexBin      string
	ClaudeBin     string
	Hook          string
	Timeout       string
}

// Path returns the config file path, respecting $PRAETOR_CONFIG.
func Path() string {
	if env := strings.TrimSpace(os.Getenv("PRAETOR_CONFIG")); env != "" {
		return env
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".praetor", "config.toml")
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
		return Config{}, err
	}

	sections, err := parse(string(data))
	if err != nil {
		return Config{}, err
	}

	global := configFromMap(sections[""])

	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return global, nil
	}

	projectSection, ok := sections[projectRoot]
	if !ok {
		return global, nil
	}
	project := configFromMap(projectSection)
	return mergeConfig(global, project), nil
}

func configFromMap(m map[string]string) Config {
	var cfg Config
	if v, ok := m["executor"]; ok {
		cfg.Executor = v
	}
	if v, ok := m["reviewer"]; ok {
		cfg.Reviewer = v
	}
	if v, ok := m["max-retries"]; ok {
		cfg.MaxRetries = intPtr(v)
	}
	if v, ok := m["max-iterations"]; ok {
		cfg.MaxIterations = intPtr(v)
	}
	if v, ok := m["isolation"]; ok {
		cfg.Isolation = v
	}
	if v, ok := m["no-review"]; ok {
		cfg.NoReview = boolPtr(v)
	}
	if v, ok := m["no-color"]; ok {
		cfg.NoColor = boolPtr(v)
	}
	if v, ok := m["codex-bin"]; ok {
		cfg.CodexBin = v
	}
	if v, ok := m["claude-bin"]; ok {
		cfg.ClaudeBin = v
	}
	if v, ok := m["hook"]; ok {
		cfg.Hook = v
	}
	if v, ok := m["timeout"]; ok {
		cfg.Timeout = v
	}
	return cfg
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
	if project.MaxRetries != nil {
		merged.MaxRetries = project.MaxRetries
	}
	if project.MaxIterations != nil {
		merged.MaxIterations = project.MaxIterations
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
	if project.Hook != "" {
		merged.Hook = project.Hook
	}
	if project.Timeout != "" {
		merged.Timeout = project.Timeout
	}
	return merged
}
