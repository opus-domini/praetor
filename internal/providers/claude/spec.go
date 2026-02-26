package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// AgentSpec knows how to build Claude CLI invocations and parse their output.
// It implements domain.AgentSpec.
type AgentSpec struct{}

// BuildCommand produces the command-line invocation for the Claude CLI.
func (a *AgentSpec) BuildCommand(req domain.AgentRequest) (domain.CommandSpec, error) {
	bin := strings.TrimSpace(req.ClaudeBin)
	if bin == "" {
		bin = "claude"
	}

	args := []string{bin, "-p",
		"--dangerously-skip-permissions",
		"--no-session-persistence",
		"--verbose",
		"--output-format", "stream-json",
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if sp := strings.TrimSpace(req.SystemPrompt); sp != "" {
		args = append(args, "--append-system-prompt", sp)
	}

	return domain.CommandSpec{
		Args:  args,
		Dir:   req.Workdir,
		Stdin: strings.TrimSpace(req.Prompt), // Claude reads prompt from stdin
	}, nil
}

// streamEvent is one NDJSON event from --output-format stream-json.
type streamEvent struct {
	Type    string  `json:"type"`
	Subtype string  `json:"subtype,omitempty"`
	Result  string  `json:"result,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// ParseOutput interprets the Claude CLI's stdout and extracts
// the usable output text and cost (if available).
func (a *AgentSpec) ParseOutput(stdout string) (string, float64, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", 0, nil
	}

	// stream-json output is NDJSON: one JSON object per line.
	// Find the last "result" event which contains the final output and cost.
	var lastResult *streamEvent
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event streamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "result" {
			e := event
			lastResult = &e
		}
	}

	if lastResult != nil {
		output := strings.TrimSpace(lastResult.Result)
		if output == "" {
			// No result text -- fall back to collecting all assistant text events.
			output = collectAssistantText(stdout)
		}
		return output, lastResult.CostUSD, nil
	}

	// No result event found -- try to collect assistant text from stream events.
	if text := collectAssistantText(stdout); text != "" {
		return text, 0, nil
	}

	// Not stream-json at all -- return raw.
	return stdout, 0, nil
}

// collectAssistantText extracts text from assistant message events in stream-json output.
func collectAssistantText(stdout string) string {
	var parts []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type != "assistant" {
			continue
		}
		for _, c := range event.Message.Content {
			if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
				parts = append(parts, strings.TrimSpace(c.Text))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// String returns a human-readable label for this agent spec.
func (a *AgentSpec) String() string { return fmt.Sprintf("claudeAgent(%s)", domain.AgentClaude) }
