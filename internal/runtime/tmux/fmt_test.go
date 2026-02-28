package tmux

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamFormatterClaudeTextDeltas(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`,
		`{"type":"result","subtype":"success","cost_usd":0.0123,"result":"Hello world"}`,
	}, "\n")

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}

	got := buf.String()
	// Should contain the streamed text, not duplicated.
	if !strings.Contains(got, "Hello world") {
		t.Errorf("expected streamed text, got:\n%s", got)
	}
	// Should contain cost summary.
	if !strings.Contains(got, "cost=$0.0123") {
		t.Errorf("expected cost summary, got:\n%s", got)
	}
	// Should NOT contain the full text twice (delta + assistant).
	count := strings.Count(got, "Hello world")
	if count != 1 {
		t.Errorf("expected text once, found %d times in:\n%s", count, got)
	}
}

func TestStreamFormatterClaudeToolUseWithArgs(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me read."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","name":"Read"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file_"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"path\":\"/src/main.go\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","name":"Bash"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"go test ./...\"}"}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","name":"Grep"}}`,
		`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"pattern\":\"func main\"}"}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Let me read."},{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Bash"},{"type":"tool_use","name":"Grep"}]}}`,
	}, "\n")

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Let me read.") {
		t.Errorf("expected text, got:\n%s", got)
	}
	if !strings.Contains(got, "[Read] /src/main.go") {
		t.Errorf("expected Read with file_path arg, got:\n%s", got)
	}
	if !strings.Contains(got, "[Bash] go test ./...") {
		t.Errorf("expected Bash with command arg, got:\n%s", got)
	}
	if !strings.Contains(got, "[Grep] func main") {
		t.Errorf("expected Grep with pattern arg, got:\n%s", got)
	}
}

func TestStreamFormatterToolUseNoArgs(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","name":"TaskList"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`{"type":"content_block_stop","index":0}`,
	}, "\n")

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[TaskList]") {
		t.Errorf("expected tool name without args, got:\n%s", got)
	}
}

func TestStreamFormatterClaudeAssistantWithToolArgs(t *testing.T) {
	t.Parallel()
	// When no deltas were streamed, the assistant message renders tools with args from input field.
	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Let me check."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/src/main.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Glob","input":{"pattern":"**/*.go"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}]}}`,
	}, "\n")

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Let me check.") {
		t.Errorf("expected text, got:\n%s", got)
	}
	if !strings.Contains(got, "[Read] /src/main.go") {
		t.Errorf("expected Read with file_path, got:\n%s", got)
	}
	if !strings.Contains(got, "[Glob] **/*.go") {
		t.Errorf("expected Glob with pattern, got:\n%s", got)
	}
	if !strings.Contains(got, "[Bash] go test ./...") {
		t.Errorf("expected Bash with command, got:\n%s", got)
	}
}

func TestStreamFormatterCodexItemCompleted(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"type":"session.start","model":"o3-mini"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"Task completed successfully.\nRESULT: PASS"}}`,
		`{"type":"session.end"}`,
	}, "\n")

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Task completed successfully.") {
		t.Errorf("expected codex output, got:\n%s", got)
	}
	if !strings.Contains(got, "RESULT: PASS") {
		t.Errorf("expected RESULT line, got:\n%s", got)
	}
}

func TestStreamFormatterPlainTextPassthrough(t *testing.T) {
	t.Parallel()
	input := "This is plain text output.\nAnother line.\n"

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "This is plain text output.") {
		t.Errorf("expected passthrough, got:\n%s", got)
	}
	if !strings.Contains(got, "Another line.") {
		t.Errorf("expected passthrough, got:\n%s", got)
	}
}

func TestStreamFormatterEmptyInput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader("")); err != nil {
		t.Fatalf("Format error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got: %q", buf.String())
	}
}

func TestStreamFormatterIgnoresUnknownEvents(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"type":"session.start","model":"o3-mini"}`,
		`{"type":"item.created","item":{"type":"agent_message"}}`,
		`{"type":"some_unknown_event","data":"stuff"}`,
		`{"type":"session.end"}`,
	}, "\n")

	var buf bytes.Buffer
	f := NewStreamFormatter(&buf)
	if err := f.Format(strings.NewReader(input)); err != nil {
		t.Fatalf("Format error: %v", err)
	}
	// Should produce no output for these events.
	if buf.Len() != 0 {
		t.Errorf("expected empty output for ignored events, got: %q", buf.String())
	}
}
