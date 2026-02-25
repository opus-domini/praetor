package config

import (
	"os"
	"path/filepath"
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
no-review = true
executor = "claude"
`
	sections, err := parse(input)
	if err != nil {
		t.Fatal(err)
	}
	cfg := configFromMap(sections[""])
	if cfg.MaxRetries == nil || *cfg.MaxRetries != 5 {
		t.Fatal("expected max-retries=5")
	}
	if cfg.NoReview == nil || *cfg.NoReview != true {
		t.Fatal("expected no-review=true")
	}
	if cfg.Executor != "claude" {
		t.Fatal("expected executor=claude")
	}
}
