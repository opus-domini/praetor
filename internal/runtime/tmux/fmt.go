package tmux

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StreamFormatter reads JSONL from an io.Reader and writes
// human-readable, ANSI-formatted text to an io.Writer.
// It auto-detects codex and claude stream-json formats.
type StreamFormatter struct {
	out       io.Writer
	hasDeltas bool // true once any content_block_delta text was written

	// Tool use accumulation: collect input_json_delta fragments
	// and display a summary when the block completes.
	toolName     string
	toolInputBuf strings.Builder
}

// NewStreamFormatter creates a formatter that writes to out.
func NewStreamFormatter(out io.Writer) *StreamFormatter {
	return &StreamFormatter{out: out}
}

// Format reads lines from r until EOF and formats each one.
func (f *StreamFormatter) Format(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Allow lines up to 1 MiB (JSONL events can be large).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		f.processLine(scanner.Text())
	}
	return scanner.Err()
}

func (f *StreamFormatter) processLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	// Fast check: if the line doesn't start with '{', it's not JSON.
	if trimmed[0] != '{' {
		_, _ = fmt.Fprintln(f.out, line)
		return
	}

	var event map[string]json.RawMessage
	if json.Unmarshal([]byte(trimmed), &event) != nil {
		// Not valid JSON — pass through as-is.
		_, _ = fmt.Fprintln(f.out, line)
		return
	}

	eventType := unquote(event["type"])
	switch eventType {
	// ── Claude stream-json events ──
	case "content_block_start":
		f.handleClaudeBlockStart(event)
	case "content_block_delta":
		f.handleClaudeDelta(event)
	case "content_block_stop":
		f.flushToolBlock()
	case "assistant":
		f.handleClaudeAssistant(event)
	case "result":
		f.handleClaudeResult(event)
	case "system":
		// Ignore init events.

	// ── Codex JSONL events ──
	case "item.completed":
		f.handleCodexItemCompleted(event)

	default:
		// Unrecognized event — silently ignore structured JSONL.
	}
}

// ── Claude handlers ──

func (f *StreamFormatter) handleClaudeBlockStart(event map[string]json.RawMessage) {
	raw, ok := event["content_block"]
	if !ok {
		return
	}
	var block struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &block) != nil {
		return
	}
	if block.Type == "tool_use" && block.Name != "" {
		if f.hasDeltas {
			_, _ = fmt.Fprintln(f.out)
			f.hasDeltas = false
		}
		f.toolName = block.Name
		f.toolInputBuf.Reset()
	}
}

// flushToolBlock renders the accumulated tool_use block (name + parsed args)
// and resets tracking state.
func (f *StreamFormatter) flushToolBlock() {
	if f.toolName == "" {
		return
	}
	summary := toolArgSummary(f.toolName, f.toolInputBuf.String())
	if summary != "" {
		_, _ = fmt.Fprintf(f.out, "\033[2m▸ [%s] %s\033[0m\n", f.toolName, summary)
	} else {
		_, _ = fmt.Fprintf(f.out, "\033[2m▸ [%s]\033[0m\n", f.toolName)
	}
	f.toolName = ""
	f.toolInputBuf.Reset()
}

// toolArgSummary extracts the most relevant argument from a tool's JSON input.
func toolArgSummary(_, inputJSON string) string {
	inputJSON = strings.TrimSpace(inputJSON)
	if inputJSON == "" {
		return ""
	}
	var args map[string]json.RawMessage
	if json.Unmarshal([]byte(inputJSON), &args) != nil {
		return ""
	}

	// Pick the most informative field per tool.
	keys := []string{
		"file_path",   // Read, Edit, Write
		"pattern",     // Glob, Grep
		"command",     // Bash
		"query",       // WebSearch, ToolSearch
		"url",         // WebFetch
		"description", // Task
		"prompt",      // Task
	}
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		val := unquote(raw)
		if val == "" {
			continue
		}
		if len(val) > 120 {
			val = val[:120] + "..."
		}
		return val
	}
	return ""
}

func (f *StreamFormatter) handleClaudeDelta(event map[string]json.RawMessage) {
	raw, ok := event["delta"]
	if !ok {
		return
	}
	var delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	}
	if json.Unmarshal(raw, &delta) != nil {
		return
	}
	switch delta.Type {
	case "text_delta":
		if delta.Text != "" {
			_, _ = fmt.Fprint(f.out, delta.Text)
			f.hasDeltas = true
		}
	case "input_json_delta":
		// Accumulate fragments; displayed on content_block_stop.
		if delta.PartialJSON != "" {
			f.toolInputBuf.WriteString(delta.PartialJSON)
		}
	}
}

func (f *StreamFormatter) handleClaudeAssistant(event map[string]json.RawMessage) {
	// If we already streamed content via deltas, skip the full message
	// to avoid duplication.
	if f.hasDeltas {
		_, _ = fmt.Fprintln(f.out)
		f.hasDeltas = false
		return
	}

	raw, ok := event["message"]
	if !ok {
		return
	}
	var msg struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return
	}
	for _, c := range msg.Content {
		switch c.Type {
		case "text":
			if text := strings.TrimSpace(c.Text); text != "" {
				_, _ = fmt.Fprintln(f.out, text)
			}
		case "tool_use":
			if c.Name != "" {
				summary := toolArgSummary(c.Name, string(c.Input))
				if summary != "" {
					_, _ = fmt.Fprintf(f.out, "\033[2m▸ [%s] %s\033[0m\n", c.Name, summary)
				} else {
					_, _ = fmt.Fprintf(f.out, "\033[2m▸ [%s]\033[0m\n", c.Name)
				}
			}
		}
	}
}

func (f *StreamFormatter) handleClaudeResult(event map[string]json.RawMessage) {
	if f.hasDeltas {
		_, _ = fmt.Fprintln(f.out)
		f.hasDeltas = false
	}

	var costUSD float64
	if raw, ok := event["cost_usd"]; ok {
		_ = json.Unmarshal(raw, &costUSD)
	}

	var parts []string
	if costUSD > 0 {
		parts = append(parts, fmt.Sprintf("cost=$%.4f", costUSD))
	}
	if len(parts) > 0 {
		_, _ = fmt.Fprintf(f.out, "\n\033[2m── %s ──\033[0m\n", strings.Join(parts, " "))
	}
}

// ── Codex handlers ──

func (f *StreamFormatter) handleCodexItemCompleted(event map[string]json.RawMessage) {
	raw, ok := event["item"]
	if !ok {
		return
	}
	var item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &item) != nil {
		return
	}
	if item.Type == "agent_message" {
		if text := strings.TrimSpace(item.Text); text != "" {
			_, _ = fmt.Fprintln(f.out, text)
		}
	}
}

// unquote extracts a JSON string value from raw bytes.
func unquote(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}
