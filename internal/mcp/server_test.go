package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestServerInitialize(t *testing.T) {
	t.Parallel()
	s := NewServer("")

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n"
	var out bytes.Buffer

	if err := s.Run(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatal("expected map result")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("expected protocol version 2024-11-05, got %v", result["protocolVersion"])
	}
}

func TestServerToolsList(t *testing.T) {
	t.Parallel()
	s := NewServer("")

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
`
	var out bytes.Buffer

	if err := s.Run(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(lines))
	}

	var toolsResp jsonRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &toolsResp); err != nil {
		t.Fatalf("decode tools response: %v", err)
	}
	if toolsResp.Error != nil {
		t.Fatalf("unexpected error: %v", toolsResp.Error)
	}
}

func TestServerPing(t *testing.T) {
	t.Parallel()
	s := NewServer("")

	input := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	var out bytes.Buffer

	if err := s.Run(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestServerUnknownMethod(t *testing.T) {
	t.Parallel()
	s := NewServer("")

	input := `{"jsonrpc":"2.0","id":1,"method":"unknown/method"}` + "\n"
	var out bytes.Buffer

	if err := s.Run(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Fatalf("expected method not found code, got %d", resp.Error.Code)
	}
}
