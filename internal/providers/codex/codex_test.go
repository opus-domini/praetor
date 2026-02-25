package codex

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestThreadIDBeforeRun(t *testing.T) {
	t.Parallel()

	thread := &Thread{}
	id, ok := thread.ID()
	if ok {
		t.Fatalf("expected ok=false before any run, got ok=true")
	}
	if id != "" {
		t.Fatalf("expected empty id before any run, got %q", id)
	}
}

func TestPatchApplyStatusConstants(t *testing.T) {
	t.Parallel()

	if PatchApplyStatusCompleted != "completed" {
		t.Fatalf("expected PatchApplyStatusCompleted=%q, got %q", "completed", PatchApplyStatusCompleted)
	}
	if PatchApplyStatusFailed != "failed" {
		t.Fatalf("expected PatchApplyStatusFailed=%q, got %q", "failed", PatchApplyStatusFailed)
	}
}

func TestNormalizeInputString(t *testing.T) {
	t.Parallel()

	prompt, images, err := normalizeInput("hello")
	if err != nil {
		t.Fatalf("normalizeInput returned error: %v", err)
	}
	if prompt != "hello" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if len(images) != 0 {
		t.Fatalf("expected no images, got %d", len(images))
	}
}

func TestNormalizeInputStructured(t *testing.T) {
	t.Parallel()

	prompt, images, err := normalizeInput([]UserInput{
		{Type: UserInputTypeText, Text: "first"},
		{Type: UserInputTypeLocalImage, Path: "/tmp/a.png"},
		{Type: UserInputTypeText, Text: "second"},
		{Type: UserInputTypeLocalImage, Path: "/tmp/b.png"},
	})
	if err != nil {
		t.Fatalf("normalizeInput returned error: %v", err)
	}
	if prompt != "first\n\nsecond" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	wantImages := []string{"/tmp/a.png", "/tmp/b.png"}
	if !reflect.DeepEqual(images, wantImages) {
		t.Fatalf("unexpected images: %#v", images)
	}
}

func TestSerializeConfigOverrides(t *testing.T) {
	t.Parallel()

	overrides, err := serializeConfigOverrides(map[string]any{
		"show_raw_agent_reasoning": true,
		"sandbox_workspace_write": map[string]any{
			"network_access": true,
		},
		"array_value": []any{"a", 2.0, false},
		"typed_array": []string{"x", "y"},
	})
	if err != nil {
		t.Fatalf("serializeConfigOverrides returned error: %v", err)
	}

	got := map[string]struct{}{}
	for _, item := range overrides {
		got[item] = struct{}{}
	}

	expected := []string{
		"array_value=[\"a\", 2, false]",
		"sandbox_workspace_write.network_access=true",
		"show_raw_agent_reasoning=true",
		"typed_array=[\"x\", \"y\"]",
	}
	for _, item := range expected {
		if _, ok := got[item]; !ok {
			t.Fatalf("expected override not found: %s (got=%v)", item, overrides)
		}
	}
}

func TestParseThreadEventItemCompleted(t *testing.T) {
	t.Parallel()

	line := []byte(`{"type":"item.completed","item":{"id":"x","type":"agent_message","text":"ok"}}`)
	event, err := parseThreadEvent(line)
	if err != nil {
		t.Fatalf("parseThreadEvent returned error: %v", err)
	}
	if event.Type != "item.completed" {
		t.Fatalf("unexpected event type: %s", event.Type)
	}
	if event.Item == nil {
		t.Fatalf("expected event item")
	}
	if event.Item.Type != "agent_message" || event.Item.Text != "ok" {
		t.Fatalf("unexpected item payload: %#v", event.Item)
	}
	if len(event.Item.Raw) == 0 {
		t.Fatalf("expected item raw payload")
	}
}

func TestParseThreadEventMcpToolCallCompleted(t *testing.T) {
	t.Parallel()

	line := []byte(`{"type":"item.completed","item":{"id":"mcp-1","type":"mcp_tool_call","server":"workspace","tool":"search","arguments":{"q":"hello"},"result":{"content":[{"type":"text","text":"ok"}],"structured_content":{"total":1}},"status":"completed"}}`)
	event, err := parseThreadEvent(line)
	if err != nil {
		t.Fatalf("parseThreadEvent returned error: %v", err)
	}
	if event.Item == nil {
		t.Fatalf("expected event item")
	}
	if event.Item.Type != "mcp_tool_call" {
		t.Fatalf("unexpected item type: %s", event.Item.Type)
	}
	if event.Item.Server != "workspace" || event.Item.Tool != "search" {
		t.Fatalf("unexpected tool payload: %#v", event.Item)
	}
	if event.Item.Result == nil {
		t.Fatalf("expected mcp tool call result payload")
	}
	if event.Item.Result.StructuredContent == nil {
		t.Fatalf("expected structured content payload")
	}
	args, ok := event.Item.Arguments.(map[string]any)
	if !ok {
		t.Fatalf("expected map arguments payload, got %T", event.Item.Arguments)
	}
	if args["q"] != "hello" {
		t.Fatalf("unexpected mcp arguments: %#v", args)
	}
}

func TestParseThreadEventTodoListAndErrorItem(t *testing.T) {
	t.Parallel()

	todoLine := []byte(`{"type":"item.updated","item":{"id":"todo-1","type":"todo_list","items":[{"text":"step one","completed":false},{"text":"step two","completed":true}]}}`)
	todoEvent, err := parseThreadEvent(todoLine)
	if err != nil {
		t.Fatalf("parseThreadEvent(todo) returned error: %v", err)
	}
	if todoEvent.Item == nil {
		t.Fatalf("expected todo item payload")
	}
	if todoEvent.Item.Type != "todo_list" {
		t.Fatalf("unexpected todo item type: %s", todoEvent.Item.Type)
	}
	if len(todoEvent.Item.Items) != 2 {
		t.Fatalf("unexpected todo list length: %d", len(todoEvent.Item.Items))
	}
	if todoEvent.Item.Items[0].Text != "step one" || todoEvent.Item.Items[0].Completed {
		t.Fatalf("unexpected todo item: %#v", todoEvent.Item.Items[0])
	}

	errorLine := []byte(`{"type":"item.completed","item":{"id":"err-1","type":"error","message":"boom"}}`)
	errorEvent, err := parseThreadEvent(errorLine)
	if err != nil {
		t.Fatalf("parseThreadEvent(error) returned error: %v", err)
	}
	if errorEvent.Item == nil {
		t.Fatalf("expected error item payload")
	}
	if errorEvent.Item.Type != "error" || errorEvent.Item.Message != "boom" {
		t.Fatalf("unexpected error item payload: %#v", errorEvent.Item)
	}
}

func TestCreateOutputSchemaFileRejectsNonObject(t *testing.T) {
	t.Parallel()

	_, err := createOutputSchemaFile([]string{"a"})
	if err == nil {
		t.Fatalf("expected error for non-object schema")
	}
}

func TestCreateOutputSchemaFileWritesJSON(t *testing.T) {
	t.Parallel()

	file, err := createOutputSchemaFile(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{"type": "string"},
		},
	})
	if err != nil {
		t.Fatalf("createOutputSchemaFile returned error: %v", err)
	}
	defer func() {
		_ = file.cleanup()
	}()

	if file.schemaPath == "" {
		t.Fatalf("expected schema path")
	}

	data, err := json.Marshal(file.schemaPath)
	if err != nil || len(data) == 0 {
		t.Fatalf("expected schema path to be serializable")
	}
}
