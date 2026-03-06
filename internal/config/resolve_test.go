package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestResolveAllDefaultsOnly(t *testing.T) {
	t.Parallel()

	resolved := ResolveAll(nil, "")
	if len(resolved) != len(Registry) {
		t.Fatalf("expected %d entries, got %d", len(Registry), len(resolved))
	}
	for _, rv := range resolved {
		if rv.Source != SourceDefault {
			t.Errorf("key %q: expected source default, got %q", rv.Key, rv.Source)
		}
	}
}

func TestResolveAllGlobalOverride(t *testing.T) {
	t.Parallel()

	sections := map[string]map[string]string{
		"": {"executor": "claude", "max-retries": "5"},
	}
	resolved := ResolveAll(sections, "")

	for _, rv := range resolved {
		switch rv.Key {
		case "executor":
			if rv.Value != "claude" || rv.Source != SourceGlobal {
				t.Errorf("executor: expected claude/config, got %q/%q", rv.Value, rv.Source)
			}
		case "max-retries":
			if rv.Value != "5" || rv.Source != SourceGlobal {
				t.Errorf("max-retries: expected 5/config, got %q/%q", rv.Value, rv.Source)
			}
		default:
			if rv.Source != SourceDefault {
				t.Errorf("key %q: expected default, got %q", rv.Key, rv.Source)
			}
		}
	}
}

func TestResolveAllProjectOverride(t *testing.T) {
	t.Parallel()

	sections := map[string]map[string]string{
		"":            {"executor": "codex", "max-retries": "5"},
		"/my/project": {"executor": "claude", "fallback-on-transient": "gemini"},
	}
	resolved := ResolveAll(sections, "/my/project")

	for _, rv := range resolved {
		switch rv.Key {
		case "executor":
			if rv.Value != "claude" || rv.Source != SourceProject {
				t.Errorf("executor: expected claude/project, got %q/%q", rv.Value, rv.Source)
			}
		case "max-retries":
			if rv.Value != "5" || rv.Source != SourceGlobal {
				t.Errorf("max-retries: expected 5/config, got %q/%q", rv.Value, rv.Source)
			}
		case "fallback-on-transient":
			if rv.Value != "gemini" || rv.Source != SourceProject {
				t.Errorf("fallback-on-transient: expected gemini/project, got %q/%q", rv.Value, rv.Source)
			}
		}
	}
}

func TestResolveAllMixedSources(t *testing.T) {
	t.Parallel()

	sections := map[string]map[string]string{
		"":            {"reviewer": "gemini"},
		"/my/project": {"hook": "./lint.sh"},
	}
	resolved := ResolveAll(sections, "/my/project")

	sources := make(map[string]Source)
	for _, rv := range resolved {
		sources[rv.Key] = rv.Source
	}

	if sources["executor"] != SourceDefault {
		t.Errorf("executor should be default, got %q", sources["executor"])
	}
	if sources["reviewer"] != SourceGlobal {
		t.Errorf("reviewer should be config, got %q", sources["reviewer"])
	}
	if sources["hook"] != SourceProject {
		t.Errorf("hook should be project, got %q", sources["hook"])
	}
}

func TestLoadResolvedMissingFile(t *testing.T) {
	t.Setenv("PRAETOR_CONFIG", filepath.Join(t.TempDir(), "nonexistent.toml"))

	resolved, path, err := LoadResolved("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
	if len(resolved) != len(Registry) {
		t.Fatalf("expected %d entries, got %d", len(Registry), len(resolved))
	}
	for _, rv := range resolved {
		if rv.Source != SourceDefault {
			t.Errorf("key %q: expected default source for missing file", rv.Key)
		}
	}
}

func TestLoadResolvedWithOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `executor = "claude"
max-retries = 7
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	resolved, _, err := LoadResolved("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, rv := range resolved {
		if rv.Key == "executor" {
			if rv.Value != "claude" || rv.Source != SourceGlobal {
				t.Errorf("executor: expected claude/config, got %q/%q", rv.Value, rv.Source)
			}
		}
		if rv.Key == "max-retries" {
			if rv.Value != "7" || rv.Source != SourceGlobal {
				t.Errorf("max-retries: expected 7/config, got %q/%q", rv.Value, rv.Source)
			}
		}
	}
}

func TestResolveAllCostPolicyDefaults(t *testing.T) {
	t.Parallel()

	resolved := ResolveAll(nil, "")
	values := make(map[string]ResolvedValue, len(resolved))
	for _, rv := range resolved {
		values[rv.Key] = rv
	}

	if values["plan-cost-budget-usd"].Value != "0" || values["plan-cost-budget-usd"].Source != SourceDefault {
		t.Fatalf("expected default plan-cost-budget-usd=0/default, got %q/%q", values["plan-cost-budget-usd"].Value, values["plan-cost-budget-usd"].Source)
	}
	if values["task-cost-budget-usd"].Value != "0" || values["task-cost-budget-usd"].Source != SourceDefault {
		t.Fatalf("expected default task-cost-budget-usd=0/default, got %q/%q", values["task-cost-budget-usd"].Value, values["task-cost-budget-usd"].Source)
	}
	if values["cost-budget-warn-threshold"].Value != "0.8" || values["cost-budget-warn-threshold"].Source != SourceDefault {
		t.Fatalf("expected default cost-budget-warn-threshold=0.8/default, got %q/%q", values["cost-budget-warn-threshold"].Value, values["cost-budget-warn-threshold"].Source)
	}
	if values["cost-budget-enforce"].Value != "true" || values["cost-budget-enforce"].Source != SourceDefault {
		t.Fatalf("expected default cost-budget-enforce=true/default, got %q/%q", values["cost-budget-enforce"].Value, values["cost-budget-enforce"].Source)
	}
}

func TestLoadResolvedCostPolicyProjectOverride(t *testing.T) {
	dir := t.TempDir()
	projectRoot := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.toml")
	content := `plan-cost-budget-usd = 5
cost-budget-warn-threshold = 0.75

[projects."` + projectRoot + `"]
task-cost-budget-usd = 1.5
cost-budget-enforce = false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	resolved, _, err := LoadResolved(projectRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values := make(map[string]ResolvedValue, len(resolved))
	for _, rv := range resolved {
		values[rv.Key] = rv
	}

	if values["plan-cost-budget-usd"].Source != SourceGlobal || values["plan-cost-budget-usd"].Value != "5" {
		t.Fatalf("expected global plan cost budget, got %q/%q", values["plan-cost-budget-usd"].Value, values["plan-cost-budget-usd"].Source)
	}
	if values["task-cost-budget-usd"].Source != SourceProject || values["task-cost-budget-usd"].Value != "1.5" {
		t.Fatalf("expected project task cost budget, got %q/%q", values["task-cost-budget-usd"].Value, values["task-cost-budget-usd"].Source)
	}
	if values["cost-budget-warn-threshold"].Source != SourceGlobal || mustParseFloat(t, values["cost-budget-warn-threshold"].Value) != 0.75 {
		t.Fatalf("expected global warn threshold 0.75/config, got %q/%q", values["cost-budget-warn-threshold"].Value, values["cost-budget-warn-threshold"].Source)
	}
	if values["cost-budget-enforce"].Source != SourceProject || values["cost-budget-enforce"].Value != "false" {
		t.Fatalf("expected project enforcement false, got %q/%q", values["cost-budget-enforce"].Value, values["cost-budget-enforce"].Source)
	}
}

func mustParseFloat(t *testing.T, raw string) float64 {
	t.Helper()

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		t.Fatalf("parse float %q: %v", raw, err)
	}
	return value
}
