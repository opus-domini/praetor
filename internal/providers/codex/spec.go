package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// AgentSpec knows how to build Codex CLI invocations and parse their output.
// It implements domain.AgentSpec.
type AgentSpec struct{}

// BuildCommand produces the command-line invocation for the Codex CLI.
func (a *AgentSpec) BuildCommand(req domain.AgentRequest) (domain.CommandSpec, error) {
	bin := strings.TrimSpace(req.CodexBin)
	if bin == "" {
		bin = "codex"
	}

	args := []string{bin, "exec", "--json",
		"--sandbox", "workspace-write",
		"--skip-git-repo-check",
		"--config", `approval_policy="never"`,
		"--cd", req.Workdir,
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}

	// Codex has no system prompt flag -- prepend to prompt.
	prompt := strings.TrimSpace(req.Prompt)
	if sp := strings.TrimSpace(req.SystemPrompt); sp != "" {
		prompt = sp + "\n\n" + prompt
	}

	// Codex reads the prompt as the final positional argument.
	args = append(args, prompt)

	return domain.CommandSpec{
		Args: args,
		Dir:  req.Workdir,
	}, nil
}

// codexStreamEvent is one JSONL event from codex exec --json.
type codexStreamEvent struct {
	Type string `json:"type"`
	Item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item,omitempty"`
	Usage struct {
		InputTokens       int `json:"input_tokens"`
		CachedInputTokens int `json:"cached_input_tokens"`
		OutputTokens      int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

// ParseOutput interprets the Codex CLI's stdout and extracts
// the usable output text and cost (if available).
func (a *AgentSpec) ParseOutput(stdout string) (string, float64, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return "", 0, nil
	}

	// --json output is JSONL: one JSON object per line.
	// Collect all agent_message texts and return them joined.
	var parts []string
	isJSONL := false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event codexStreamEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "" {
			continue
		}
		isJSONL = true
		if event.Type == "item.completed" && event.Item.Type == "agent_message" {
			if text := strings.TrimSpace(event.Item.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}

	if isJSONL && len(parts) > 0 {
		return strings.Join(parts, "\n"), 0, nil
	}

	// Not JSONL -- return raw.
	return stdout, 0, nil
}

// String returns a human-readable label for this agent spec.
func (a *AgentSpec) String() string { return fmt.Sprintf("codexAgent(%s)", domain.AgentCodex) }
