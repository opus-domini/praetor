package loop

import (
	"fmt"
	"strings"
)

// claudeAgent knows how to build Claude CLI invocations and parse their output.
type claudeAgent struct{}

func (a *claudeAgent) BuildCommand(req AgentRequest) (CommandSpec, error) {
	bin := strings.TrimSpace(req.ClaudeBin)
	if bin == "" {
		bin = "claude"
	}

	args := []string{bin, "-p",
		"--dangerously-skip-permissions",
		"--no-session-persistence",
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if req.Verbose {
		args = append(args, "--verbose")
	}
	if sp := strings.TrimSpace(req.SystemPrompt); sp != "" {
		args = append(args, "--append-system-prompt", sp)
	}

	return CommandSpec{
		Args:  args,
		Dir:   req.Workdir,
		Stdin: strings.TrimSpace(req.Prompt), // Claude reads prompt from stdin
	}, nil
}

func (a *claudeAgent) ParseOutput(stdout string) (string, float64, error) {
	return strings.TrimSpace(stdout), 0, nil // Claude cost comes from SDK, not CLI stdout
}

func (a *claudeAgent) String() string { return fmt.Sprintf("claudeAgent(%s)", AgentClaude) }
