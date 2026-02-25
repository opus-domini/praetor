package loop

import (
	"encoding/json"
	"fmt"
	"strings"
)

// codexAgent knows how to build Codex CLI invocations and parse their output.
type codexAgent struct{}

func (a *codexAgent) BuildCommand(req AgentRequest) (CommandSpec, error) {
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

	// Codex has no system prompt flag — prepend to prompt.
	prompt := strings.TrimSpace(req.Prompt)
	if sp := strings.TrimSpace(req.SystemPrompt); sp != "" {
		prompt = sp + "\n\n" + prompt
	}

	// Codex reads the prompt as the final positional argument.
	args = append(args, prompt)

	return CommandSpec{
		Args: args,
		Dir:  req.Workdir,
	}, nil
}

func (a *codexAgent) ParseOutput(stdout string) (string, float64, error) {
	stdout = strings.TrimSpace(stdout)
	if !strings.HasPrefix(stdout, "{") {
		return stdout, 0, nil
	}

	var parsed struct {
		Result       string  `json:"result"`
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		return stdout, 0, nil //nolint:nilerr // invalid JSON is not an error — return raw output
	}

	output := stdout
	if parsed.Result != "" {
		output = parsed.Result
	}
	return output, parsed.TotalCostUSD, nil
}

func (a *codexAgent) String() string { return fmt.Sprintf("codexAgent(%s)", AgentCodex) }
