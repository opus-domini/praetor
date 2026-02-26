package agents

import "testing"

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
