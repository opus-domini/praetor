package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("PRAETOR_CONFIG", filepath.Join(t.TempDir(), "nonexistent.toml"))

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Executor != "" {
		t.Fatalf("expected zero config, got executor=%q", cfg.Executor)
	}
}

func TestLoadGlobalDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
executor = "claude"
reviewer = "claude"
max-retries = 5
no-review = false
isolation = "worktree"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Executor != "claude" {
		t.Fatalf("expected executor=claude, got %q", cfg.Executor)
	}
	if cfg.Reviewer != "claude" {
		t.Fatalf("expected reviewer=claude, got %q", cfg.Reviewer)
	}
	if cfg.MaxRetries == nil || *cfg.MaxRetries != 5 {
		t.Fatalf("expected max-retries=5")
	}
	if cfg.NoReview == nil || *cfg.NoReview != false {
		t.Fatalf("expected no-review=false")
	}
	if cfg.Isolation != "worktree" {
		t.Fatalf("expected isolation=worktree, got %q", cfg.Isolation)
	}
}

func TestLoadExtendedProviderSettings(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
copilot-bin = "gh-copilot"
kimi-bin = "/usr/local/bin/kimi"
opencode-bin = "opencode"
openrouter-url = "https://openrouter.example/v1"
openrouter-model = "openai/gpt-4o-mini"
openrouter-api-key-env = "OPENROUTER_TOKEN"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CopilotBin != "gh-copilot" {
		t.Fatalf("expected copilot-bin, got %q", cfg.CopilotBin)
	}
	if cfg.KimiBin != "/usr/local/bin/kimi" {
		t.Fatalf("expected kimi-bin, got %q", cfg.KimiBin)
	}
	if cfg.OpenCodeBin != "opencode" {
		t.Fatalf("expected opencode-bin, got %q", cfg.OpenCodeBin)
	}
	if cfg.OpenRouterURL != "https://openrouter.example/v1" {
		t.Fatalf("expected openrouter-url, got %q", cfg.OpenRouterURL)
	}
	if cfg.OpenRouterModel != "openai/gpt-4o-mini" {
		t.Fatalf("expected openrouter-model, got %q", cfg.OpenRouterModel)
	}
	if cfg.OpenRouterKeyEnv != "OPENROUTER_TOKEN" {
		t.Fatalf("expected openrouter-api-key-env, got %q", cfg.OpenRouterKeyEnv)
	}
}

func TestLoadProjectOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
executor = "codex"
max-retries = 3

[projects."/my/project"]
executor = "claude"
hook = "./lint.sh"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	cfg, err := Load("/my/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Executor != "claude" {
		t.Fatalf("expected project override executor=claude, got %q", cfg.Executor)
	}
	if cfg.Hook != "./lint.sh" {
		t.Fatalf("expected hook=./lint.sh, got %q", cfg.Hook)
	}
	// Global default preserved when project doesn't override
	if cfg.MaxRetries == nil || *cfg.MaxRetries != 3 {
		t.Fatalf("expected max-retries=3 from global")
	}
}

func TestLoadNoProjectMatch(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
executor = "codex"

[projects."/other/project"]
executor = "claude"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	cfg, err := Load("/my/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Executor != "codex" {
		t.Fatalf("expected global executor=codex, got %q", cfg.Executor)
	}
}

func TestLoadProjectSectionMatchesNormalizedPath(t *testing.T) {
	dir := t.TempDir()
	projectRoot := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(dir, "project-link")
	if err := os.Symlink(projectRoot, symlinkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	cfgPath := filepath.Join(dir, "config.toml")
	content := `
executor = "codex"

[projects."` + projectRoot + `"]
executor = "claude"
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	cfg, err := Load(symlinkPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Executor != "claude" {
		t.Fatalf("expected normalized project override executor=claude, got %q", cfg.Executor)
	}
}

func TestParserComments(t *testing.T) {
	t.Parallel()
	input := `# This is a comment
executor = "claude"
# Another comment
reviewer = "codex"
`
	sections, err := parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if sections[""]["executor"] != "claude" {
		t.Fatalf("expected executor=claude")
	}
	if sections[""]["reviewer"] != "codex" {
		t.Fatalf("expected reviewer=codex")
	}
}

func TestParserTypes(t *testing.T) {
	t.Parallel()
	input := `
max-retries = 5
max-transitions = 100
keep-last-runs = 20
no-review = true
executor = "claude"
`
	sections, err := parse(input)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := configFromMap("global", sections[""])
	if err != nil {
		t.Fatalf("unexpected parse config error: %v", err)
	}
	if cfg.MaxRetries == nil || *cfg.MaxRetries != 5 {
		t.Fatal("expected max-retries=5")
	}
	if cfg.NoReview == nil || *cfg.NoReview != true {
		t.Fatal("expected no-review=true")
	}
	if cfg.MaxTransitions == nil || *cfg.MaxTransitions != 100 {
		t.Fatal("expected max-transitions=100")
	}
	if cfg.KeepLastRuns == nil || *cfg.KeepLastRuns != 20 {
		t.Fatal("expected keep-last-runs=20")
	}
	if cfg.Executor != "claude" {
		t.Fatal("expected executor=claude")
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
executor = "codex"
unknown-key = "value"
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	_, err := Load("")
	if err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestLoadRejectsDuplicatedKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
executor = "codex"
executor = "claude"
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	_, err := Load("")
	if err == nil {
		t.Fatal("expected duplicated key error")
	}
}

func TestLoadRejectsUnknownSection(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults]
executor = "codex"
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	_, err := Load("")
	if err == nil {
		t.Fatal("expected unknown section error")
	}
}

func TestLoadRejectsInvalidTypes(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
max-retries = "abc"
no-review = "maybe"
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	_, err := Load("")
	if err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
timeout = "-5m"
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	_, err := Load("")
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
}

func TestLoadErrorIncludesPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
this is invalid
	`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	_, err := Load("")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), cfgPath) {
		t.Fatalf("expected error to include config path %q, got %v", cfgPath, err)
	}
}
