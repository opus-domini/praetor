package adapters

import "testing"

func TestParseCodexOutputFromJSONL(t *testing.T) {
	t.Parallel()

	// Real codex --json output (no model in events)
	stdout := `{"type":"thread.started","thread_id":"019c9b80-fa16-7912-acab-d19e3def915b"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"reasoning","text":"**Replying with confirmation**"}}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"OK"}}
{"type":"turn.completed","usage":{"input_tokens":13472,"cached_input_tokens":3456,"output_tokens":102}}`

	got := parseCodexOutput(stdout)
	if got.Output != "OK" {
		t.Errorf("Output = %q, want %q", got.Output, "OK")
	}
	if got.Model != "" {
		t.Errorf("Model = %q, want empty (codex --json does not expose model)", got.Model)
	}
}

func TestParseCodexOutputExtractsModelIfPresent(t *testing.T) {
	t.Parallel()

	// Hypothetical future codex output with model field
	stdout := `{"type":"thread.started","thread_id":"abc","model":"gpt-5.3-codex"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Hi"}}
{"type":"turn.completed"}`

	got := parseCodexOutput(stdout)
	if got.Output != "Hi" {
		t.Errorf("Output = %q, want %q", got.Output, "Hi")
	}
	if got.Model != "gpt-5.3-codex" {
		t.Errorf("Model = %q, want %q", got.Model, "gpt-5.3-codex")
	}
}

func TestParseCodexOutputPlainText(t *testing.T) {
	t.Parallel()

	got := parseCodexOutput("just plain text")
	if got.Output != "just plain text" {
		t.Errorf("Output = %q, want %q", got.Output, "just plain text")
	}
}

func TestParseCodexOutputEmpty(t *testing.T) {
	t.Parallel()

	got := parseCodexOutput("")
	if got.Output != "" {
		t.Errorf("Output = %q, want empty", got.Output)
	}
}
