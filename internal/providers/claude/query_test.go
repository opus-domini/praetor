package claude

import (
	"encoding/json"
	"testing"
)

func TestParseAssistantMessage(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		parentID := "tool-123"
		raw := mustMarshal(t, map[string]any{
			"type":               "assistant",
			"uuid":               "a-uuid",
			"session_id":         "sess-1",
			"parent_tool_use_id": parentID,
			"message":            map[string]any{"role": "assistant"},
		})
		msg := SDKMessage{Type: "assistant", Raw: raw}

		out, err := ParseAssistantMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Type != "assistant" {
			t.Fatalf("expected type assistant, got %q", out.Type)
		}
		if out.UUID != "a-uuid" {
			t.Fatalf("expected uuid a-uuid, got %q", out.UUID)
		}
		if out.SessionID != "sess-1" {
			t.Fatalf("expected session_id sess-1, got %q", out.SessionID)
		}
		if out.ParentToolUseID == nil || *out.ParentToolUseID != parentID {
			t.Fatalf("expected parent_tool_use_id %q, got %v", parentID, out.ParentToolUseID)
		}
		if len(out.Message) == 0 {
			t.Fatal("expected non-empty message")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()
		msg := SDKMessage{Type: "system", Raw: json.RawMessage(`{}`)}
		_, err := ParseAssistantMessage(msg)
		if err == nil {
			t.Fatal("expected error for wrong type")
		}
	})
}

func TestParseSystemInitMessage(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		raw := mustMarshal(t, map[string]any{
			"type":                "system",
			"subtype":             "init",
			"model":               "claude-sonnet-4-20250514",
			"cwd":                 "/home/user/project",
			"tools":               []string{"Bash", "Read"},
			"permissionMode":      "default",
			"claude_code_version": "1.0.0",
			"uuid":                "init-uuid",
			"session_id":          "sess-init",
		})
		msg := SDKMessage{Type: "system", Raw: raw}

		out, err := ParseSystemInitMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Type != "system" {
			t.Fatalf("expected type system, got %q", out.Type)
		}
		if out.Subtype != "init" {
			t.Fatalf("expected subtype init, got %q", out.Subtype)
		}
		if out.Model != "claude-sonnet-4-20250514" {
			t.Fatalf("expected model claude-sonnet-4-20250514, got %q", out.Model)
		}
		if out.CWD != "/home/user/project" {
			t.Fatalf("expected cwd /home/user/project, got %q", out.CWD)
		}
		if len(out.Tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(out.Tools))
		}
		if out.PermissionMode != "default" {
			t.Fatalf("expected permissionMode default, got %q", out.PermissionMode)
		}
		if out.ClaudeVersion != "1.0.0" {
			t.Fatalf("expected claude_code_version 1.0.0, got %q", out.ClaudeVersion)
		}
		if out.UUID != "init-uuid" {
			t.Fatalf("expected uuid init-uuid, got %q", out.UUID)
		}
		if out.SessionID != "sess-init" {
			t.Fatalf("expected session_id sess-init, got %q", out.SessionID)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()
		msg := SDKMessage{Type: "assistant", Raw: json.RawMessage(`{}`)}
		_, err := ParseSystemInitMessage(msg)
		if err == nil {
			t.Fatal("expected error for wrong type")
		}
	})

	t.Run("wrong subtype", func(t *testing.T) {
		t.Parallel()
		raw := mustMarshal(t, map[string]any{
			"type":    "system",
			"subtype": "status",
		})
		msg := SDKMessage{Type: "system", Raw: raw}
		_, err := ParseSystemInitMessage(msg)
		if err == nil {
			t.Fatal("expected error for wrong subtype")
		}
	})
}

func TestParseStatusMessage(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		raw := mustMarshal(t, map[string]any{
			"type":           "system",
			"subtype":        "status",
			"status":         "compacting",
			"permissionMode": "plan",
		})
		msg := SDKMessage{Type: "system", Raw: raw}

		out, err := ParseStatusMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Type != "system" {
			t.Fatalf("expected type system, got %q", out.Type)
		}
		if out.Subtype != "status" {
			t.Fatalf("expected subtype status, got %q", out.Subtype)
		}
		if out.Status != "compacting" {
			t.Fatalf("expected status compacting, got %q", out.Status)
		}
		if out.PermissionMode != "plan" {
			t.Fatalf("expected permissionMode plan, got %q", out.PermissionMode)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()
		msg := SDKMessage{Type: "result", Raw: json.RawMessage(`{}`)}
		_, err := ParseStatusMessage(msg)
		if err == nil {
			t.Fatal("expected error for wrong type")
		}
	})

	t.Run("wrong subtype", func(t *testing.T) {
		t.Parallel()
		raw := mustMarshal(t, map[string]any{
			"type":    "system",
			"subtype": "init",
		})
		msg := SDKMessage{Type: "system", Raw: raw}
		_, err := ParseStatusMessage(msg)
		if err == nil {
			t.Fatal("expected error for wrong subtype")
		}
	})
}

func TestParseResultMessageFullPayload(t *testing.T) {
	t.Parallel()

	stopReason := "end_turn"
	raw := mustMarshal(t, map[string]any{
		"type":            "result",
		"subtype":         "success",
		"is_error":        false,
		"result":          "Task completed successfully.",
		"session_id":      "sess-result",
		"uuid":            "result-uuid",
		"duration_ms":     12345,
		"duration_api_ms": 6789,
		"num_turns":       3,
		"stop_reason":     stopReason,
		"total_cost_usd":  0.0542,
		"usage": map[string]any{
			"input_tokens":                1000,
			"output_tokens":               500,
			"cache_read_input_tokens":     200,
			"cache_creation_input_tokens": 100,
		},
		"modelUsage": map[string]any{
			"claude-sonnet-4-20250514": map[string]any{
				"inputTokens":              800,
				"outputTokens":             400,
				"cacheReadInputTokens":     150,
				"cacheCreationInputTokens": 50,
				"webSearchRequests":        2,
				"costUSD":                  0.0542,
				"contextWindow":            200000,
				"maxOutputTokens":          8192,
			},
		},
		"permission_denials": []any{
			map[string]any{
				"tool_name":   "Bash",
				"tool_use_id": "tu-1",
				"tool_input":  map[string]any{"command": "rm -rf /"},
			},
		},
		"structured_output": map[string]any{"answer": 42},
		"errors":            []any{"warning: token limit near"},
	})
	msg := SDKMessage{Type: "result", Raw: raw}

	out, err := ParseResultMessage(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Type != "result" {
		t.Fatalf("expected type result, got %q", out.Type)
	}
	if out.Subtype != "success" {
		t.Fatalf("expected subtype success, got %q", out.Subtype)
	}
	if out.IsError {
		t.Fatal("expected is_error to be false")
	}
	if out.Result != "Task completed successfully." {
		t.Fatalf("unexpected result: %q", out.Result)
	}
	if out.SessionID != "sess-result" {
		t.Fatalf("expected session_id sess-result, got %q", out.SessionID)
	}
	if out.UUID != "result-uuid" {
		t.Fatalf("expected uuid result-uuid, got %q", out.UUID)
	}
	if out.DurationMS != 12345 {
		t.Fatalf("expected duration_ms 12345, got %d", out.DurationMS)
	}
	if out.DurationAPIMS != 6789 {
		t.Fatalf("expected duration_api_ms 6789, got %d", out.DurationAPIMS)
	}
	if out.NumTurns != 3 {
		t.Fatalf("expected num_turns 3, got %d", out.NumTurns)
	}
	if out.StopReason == nil || *out.StopReason != stopReason {
		t.Fatalf("expected stop_reason %q, got %v", stopReason, out.StopReason)
	}
	if out.TotalCostUSD != 0.0542 {
		t.Fatalf("expected total_cost_usd 0.0542, got %f", out.TotalCostUSD)
	}
	if out.Usage == nil {
		t.Fatal("expected usage to be non-nil")
	}
	if out.Usage.InputTokens != 1000 {
		t.Fatalf("expected usage.input_tokens 1000, got %d", out.Usage.InputTokens)
	}
	if out.Usage.OutputTokens != 500 {
		t.Fatalf("expected usage.output_tokens 500, got %d", out.Usage.OutputTokens)
	}
	if out.Usage.CacheReadInputTokens != 200 {
		t.Fatalf("expected usage.cache_read_input_tokens 200, got %d", out.Usage.CacheReadInputTokens)
	}
	if out.Usage.CacheCreationInputTokens != 100 {
		t.Fatalf("expected usage.cache_creation_input_tokens 100, got %d", out.Usage.CacheCreationInputTokens)
	}

	mu, ok := out.ModelUsage["claude-sonnet-4-20250514"]
	if !ok {
		t.Fatal("expected modelUsage entry for claude-sonnet-4-20250514")
	}
	if mu.InputTokens != 800 {
		t.Fatalf("expected modelUsage inputTokens 800, got %d", mu.InputTokens)
	}
	if mu.OutputTokens != 400 {
		t.Fatalf("expected modelUsage outputTokens 400, got %d", mu.OutputTokens)
	}
	if mu.CacheReadInputTokens != 150 {
		t.Fatalf("expected modelUsage cacheReadInputTokens 150, got %d", mu.CacheReadInputTokens)
	}
	if mu.CacheCreationInputTokens != 50 {
		t.Fatalf("expected modelUsage cacheCreationInputTokens 50, got %d", mu.CacheCreationInputTokens)
	}
	if mu.WebSearchRequests != 2 {
		t.Fatalf("expected modelUsage webSearchRequests 2, got %d", mu.WebSearchRequests)
	}
	if mu.CostUSD != 0.0542 {
		t.Fatalf("expected modelUsage costUSD 0.0542, got %f", mu.CostUSD)
	}
	if mu.ContextWindow != 200000 {
		t.Fatalf("expected modelUsage contextWindow 200000, got %d", mu.ContextWindow)
	}
	if mu.MaxOutputTokens != 8192 {
		t.Fatalf("expected modelUsage maxOutputTokens 8192, got %d", mu.MaxOutputTokens)
	}

	if len(out.PermissionDenials) != 1 {
		t.Fatalf("expected 1 permission denial, got %d", len(out.PermissionDenials))
	}
	pd := out.PermissionDenials[0]
	if pd.ToolName != "Bash" {
		t.Fatalf("expected permission denial tool_name Bash, got %q", pd.ToolName)
	}
	if pd.ToolUseID != "tu-1" {
		t.Fatalf("expected permission denial tool_use_id tu-1, got %q", pd.ToolUseID)
	}
	if cmd, ok := pd.ToolInput["command"]; !ok || cmd != "rm -rf /" {
		t.Fatalf("expected permission denial tool_input command 'rm -rf /', got %v", pd.ToolInput)
	}

	if len(out.StructuredOutput) == 0 {
		t.Fatal("expected non-empty structured_output")
	}

	if len(out.Errors) != 1 || out.Errors[0] != "warning: token limit near" {
		t.Fatalf("expected errors [warning: token limit near], got %v", out.Errors)
	}

	if len(out.Raw) == 0 {
		t.Fatal("expected Raw to be preserved")
	}
}

func TestParseResultMessageMinimalPayload(t *testing.T) {
	t.Parallel()

	raw := mustMarshal(t, map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"result":     "done",
		"session_id": "sess-min",
	})
	msg := SDKMessage{Type: "result", Raw: raw}

	out, err := ParseResultMessage(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Type != "result" {
		t.Fatalf("expected type result, got %q", out.Type)
	}
	if out.Subtype != "success" {
		t.Fatalf("expected subtype success, got %q", out.Subtype)
	}
	if out.IsError {
		t.Fatal("expected is_error to be false")
	}
	if out.Result != "done" {
		t.Fatalf("expected result done, got %q", out.Result)
	}
	if out.SessionID != "sess-min" {
		t.Fatalf("expected session_id sess-min, got %q", out.SessionID)
	}

	// New fields should be zero-valued.
	if out.UUID != "" {
		t.Fatalf("expected empty uuid, got %q", out.UUID)
	}
	if out.DurationMS != 0 {
		t.Fatalf("expected duration_ms 0, got %d", out.DurationMS)
	}
	if out.DurationAPIMS != 0 {
		t.Fatalf("expected duration_api_ms 0, got %d", out.DurationAPIMS)
	}
	if out.NumTurns != 0 {
		t.Fatalf("expected num_turns 0, got %d", out.NumTurns)
	}
	if out.StopReason != nil {
		t.Fatalf("expected nil stop_reason, got %v", out.StopReason)
	}
	if out.TotalCostUSD != 0 {
		t.Fatalf("expected total_cost_usd 0, got %f", out.TotalCostUSD)
	}
	if out.Usage != nil {
		t.Fatalf("expected nil usage, got %v", out.Usage)
	}
	if out.ModelUsage != nil {
		t.Fatalf("expected nil modelUsage, got %v", out.ModelUsage)
	}
	if out.PermissionDenials != nil {
		t.Fatalf("expected nil permission_denials, got %v", out.PermissionDenials)
	}
	if out.StructuredOutput != nil {
		t.Fatalf("expected nil structured_output, got %v", out.StructuredOutput)
	}
	if out.Errors != nil {
		t.Fatalf("expected nil errors, got %v", out.Errors)
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal test fixture: %v", err)
	}
	return data
}
