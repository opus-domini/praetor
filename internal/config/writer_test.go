package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetValueCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sub", "config.toml")

	if err := SetValue(cfgPath, "", "executor", "claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `executor = "claude"`) {
		t.Errorf("expected executor=claude in file, got:\n%s", data)
	}
}

func TestSetValueUpdatesExistingKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	initial := `executor = "codex"
reviewer = "claude"
`
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetValue(cfgPath, "", "executor", "claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `executor = "claude"`) {
		t.Errorf("expected updated executor, got:\n%s", content)
	}
	if !strings.Contains(content, `reviewer = "claude"`) {
		t.Errorf("expected reviewer preserved, got:\n%s", content)
	}
}

func TestSetValuePreservesComments(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	initial := `# My config
executor = "codex"
# Keep this comment
reviewer = "claude"
`
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetValue(cfgPath, "", "executor", "gemini"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# My config") {
		t.Errorf("first comment lost:\n%s", content)
	}
	if !strings.Contains(content, "# Keep this comment") {
		t.Errorf("second comment lost:\n%s", content)
	}
}

func TestSetValueAppendsNewKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	initial := `executor = "codex"
`
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetValue(cfgPath, "", "reviewer", "claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `reviewer = "claude"`) {
		t.Errorf("expected appended reviewer, got:\n%s", content)
	}
}

func TestSetValueRejectsUnknownKey(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	err := SetValue(cfgPath, "", "unknown-key", "value")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("expected unknown key error, got: %v", err)
	}
}

func TestSetValueRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	err := SetValue(cfgPath, "", "max-retries", "abc")
	if err == nil {
		t.Fatal("expected error for invalid integer")
	}
}

func TestSetValueProjectSection(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	initial := `executor = "codex"
`
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetValue(cfgPath, "/my/project", "executor", "claude"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `[projects."/my/project"]`) {
		t.Errorf("expected project section header, got:\n%s", content)
	}
	if !strings.Contains(content, `executor = "claude"`) {
		t.Errorf("expected project executor, got:\n%s", content)
	}
}

func TestSetValueProjectSectionAppend(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	initial := `executor = "codex"

[projects."/my/project"]
executor = "claude"
`
	if err := os.WriteFile(cfgPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetValue(cfgPath, "/my/project", "reviewer", "gemini"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `reviewer = "gemini"`) {
		t.Errorf("expected appended reviewer in project section, got:\n%s", content)
	}
}

func TestSetValueRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	if err := SetValue(cfgPath, "", "executor", "claude"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := SetValue(cfgPath, "", "max-retries", "7"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Executor != "claude" {
		t.Errorf("expected executor=claude, got %q", cfg.Executor)
	}
	if cfg.MaxRetries == nil || *cfg.MaxRetries != 7 {
		t.Errorf("expected max-retries=7")
	}
}

func TestSetValueIntFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	if err := SetValue(cfgPath, "", "max-retries", "5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "max-retries = 5") {
		t.Errorf("expected bare integer, got:\n%s", data)
	}
}

func TestSetValueBoolFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	if err := SetValue(cfgPath, "", "no-review", "true"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "no-review = true") {
		t.Errorf("expected bare boolean, got:\n%s", data)
	}
}

func TestSetValueFloatFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	if err := SetValue(cfgPath, "", "plan-cost-budget-usd", "12.5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := SetValue(cfgPath, "", "cost-budget-warn-threshold", "0.9"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "plan-cost-budget-usd = 12.5") {
		t.Errorf("expected bare float plan budget, got:\n%s", content)
	}
	if !strings.Contains(content, "cost-budget-warn-threshold = 0.9") {
		t.Errorf("expected bare float warn threshold, got:\n%s", content)
	}
}

func TestSetValueCostPolicyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	t.Setenv("PRAETOR_CONFIG", cfgPath)

	if err := SetValue(cfgPath, "", "plan-cost-budget-usd", "10"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := SetValue(cfgPath, "", "task-cost-budget-usd", "2.5"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := SetValue(cfgPath, "", "cost-budget-warn-threshold", "0.85"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := SetValue(cfgPath, "", "cost-budget-enforce", "false"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PlanCostBudgetUSD == nil || *cfg.PlanCostBudgetUSD != 10 {
		t.Errorf("expected plan-cost-budget-usd=10, got %#v", cfg.PlanCostBudgetUSD)
	}
	if cfg.TaskCostBudgetUSD == nil || *cfg.TaskCostBudgetUSD != 2.5 {
		t.Errorf("expected task-cost-budget-usd=2.5, got %#v", cfg.TaskCostBudgetUSD)
	}
	if cfg.CostWarnThreshold == nil || *cfg.CostWarnThreshold != 0.85 {
		t.Errorf("expected cost-budget-warn-threshold=0.85, got %#v", cfg.CostWarnThreshold)
	}
	if cfg.CostBudgetEnforce == nil || *cfg.CostBudgetEnforce {
		t.Errorf("expected cost-budget-enforce=false, got %#v", cfg.CostBudgetEnforce)
	}
}

func TestValidateValueRejectsNegativeRetries(t *testing.T) {
	t.Parallel()

	err := ValidateValue("max-retries", "0")
	if err == nil {
		t.Error("expected error for zero max-retries")
	}
}

func TestValidateValueRejectsNegativeDuration(t *testing.T) {
	t.Parallel()

	err := ValidateValue("timeout", "-5m")
	if err == nil {
		t.Error("expected error for negative timeout")
	}
}

func TestValidateValueAcceptsValidDuration(t *testing.T) {
	t.Parallel()

	if err := ValidateValue("timeout", "30m"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateValueAcceptsString(t *testing.T) {
	t.Parallel()

	if err := ValidateValue("executor", "any-value"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateValueRejectsNegativeCostBudget(t *testing.T) {
	t.Parallel()

	err := ValidateValue("plan-cost-budget-usd", "-1")
	if err == nil {
		t.Fatal("expected error for negative cost budget")
	}
}

func TestValidateValueRejectsInvalidCostWarnThreshold(t *testing.T) {
	t.Parallel()

	err := ValidateValue("cost-budget-warn-threshold", "1.1")
	if err == nil {
		t.Fatal("expected error for warn threshold above 1")
	}
}
