package adapters

import (
	"strings"
	"testing"
)

func TestParseClaudeOutputJSON(t *testing.T) {
	t.Parallel()

	stdout := `{"result":"OK","model":"claude-sonnet-4-20250514","cost_usd":0.0042,"session_id":"abc"}`

	got := parseClaudeOutput(stdout)
	if got.Output != "OK" {
		t.Errorf("Output = %q, want %q", got.Output, "OK")
	}
	if got.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", got.Model, "claude-sonnet-4-20250514")
	}
	if got.CostUSD != 0.0042 {
		t.Errorf("CostUSD = %v, want %v", got.CostUSD, 0.0042)
	}
}

func TestParseClaudeOutputStructuredOutputEnvelope(t *testing.T) {
	t.Parallel()

	stdout := `{"type":"result","cost_usd":0.0042,"structured_output":{"name":"test-plan","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}}`

	got := parseClaudeOutput(stdout)
	if got.Output == stdout {
		t.Fatal("expected structured_output to be extracted from envelope")
	}
	if got.CostUSD != 0.0042 {
		t.Errorf("CostUSD = %v, want %v", got.CostUSD, 0.0042)
	}
	if want := `"name":"test-plan"`; !strings.Contains(got.Output, want) {
		t.Fatalf("Output = %q, want to contain %q", got.Output, want)
	}
}

func TestParseClaudeOutputPrefersStructuredOutputOverResultText(t *testing.T) {
	t.Parallel()

	stdout := `{"type":"result","result":"I made assumptions and here is a summary.","cost_usd":0.0042,"structured_output":{"name":"test-plan","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}}`

	got := parseClaudeOutput(stdout)
	if strings.Contains(got.Output, "summary") {
		t.Fatalf("Output should prefer structured_output over result text, got %q", got.Output)
	}
	if want := `"name":"test-plan"`; !strings.Contains(got.Output, want) {
		t.Fatalf("Output = %q, want to contain %q", got.Output, want)
	}
}

func TestParseClaudeOutputEventArrayUsesStructuredOutputFromResult(t *testing.T) {
	t.Parallel()

	stdout := `[{"type":"system","subtype":"init"},{"type":"assistant","message":{"content":[{"type":"text","text":"ignore me"}]}},{"type":"result","cost_usd":0.0042,"structured_output":{"name":"test-plan","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}}]`

	got := parseClaudeOutput(stdout)
	if got.Output == stdout {
		t.Fatal("expected result event to be extracted from event array")
	}
	if got.CostUSD != 0.0042 {
		t.Errorf("CostUSD = %v, want %v", got.CostUSD, 0.0042)
	}
	if want := `"name":"test-plan"`; !strings.Contains(got.Output, want) {
		t.Fatalf("Output = %q, want to contain %q", got.Output, want)
	}
}

func TestParseClaudeOutputJSONNoModel(t *testing.T) {
	t.Parallel()

	stdout := `{"result":"Hello","cost_usd":0.01}`

	got := parseClaudeOutput(stdout)
	if got.Output != "Hello" {
		t.Errorf("Output = %q, want %q", got.Output, "Hello")
	}
	if got.Model != "" {
		t.Errorf("Model = %q, want empty", got.Model)
	}
}

func TestParseClaudeOutputEmpty(t *testing.T) {
	t.Parallel()

	got := parseClaudeOutput("")
	if got.Output != "" {
		t.Errorf("Output = %q, want empty", got.Output)
	}
}

func TestParseClaudeOutputPlainTextFallback(t *testing.T) {
	t.Parallel()

	got := parseClaudeOutput("just plain text, not JSON")
	if got.Output != "just plain text, not JSON" {
		t.Errorf("Output = %q, want %q", got.Output, "just plain text, not JSON")
	}
	if got.Model != "" {
		t.Errorf("Model = %q, want empty", got.Model)
	}
}
