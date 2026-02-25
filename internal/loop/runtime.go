package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// composedRuntime implements AgentRuntime by composing AgentSpec and ProcessRunner.
type composedRuntime struct {
	agents map[Agent]AgentSpec
	runner ProcessRunner
}

func newComposedRuntime(agents map[Agent]AgentSpec, runner ProcessRunner) *composedRuntime {
	return &composedRuntime{agents: agents, runner: runner}
}

func (r *composedRuntime) Run(ctx context.Context, req AgentRequest) (AgentResult, error) {
	start := time.Now()

	agent, ok := r.agents[normalizeAgent(req.Agent)]
	if !ok {
		return AgentResult{}, fmt.Errorf("unsupported agent %q", req.Agent)
	}

	spec, err := agent.BuildCommand(req)
	if err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, err
	}
	spec.WindowHint = strings.TrimSpace(req.TaskLabel)

	// Persist prompt and system prompt files for debugging.
	if runDir := strings.TrimSpace(req.RunDir); runDir != "" {
		prefix := strings.TrimSpace(req.OutputPrefix)
		if prefix == "" {
			prefix = "agent"
		}
		_ = os.WriteFile(filepath.Join(runDir, prefix+".prompt"), []byte(strings.TrimSpace(req.Prompt)), 0o644)
		_ = os.WriteFile(filepath.Join(runDir, prefix+".system-prompt"), []byte(strings.TrimSpace(req.SystemPrompt)), 0o644)
	}

	result, err := r.runner.Run(ctx, spec, req.RunDir, req.OutputPrefix)
	if err != nil {
		return AgentResult{DurationS: time.Since(start).Seconds()}, err
	}

	if result.ExitCode != 0 {
		return AgentResult{
				Output:    result.Stdout,
				DurationS: time.Since(start).Seconds(),
			}, fmt.Errorf("agent process failed with exit code %d: %s",
				result.ExitCode, tailText(result.Stderr, 20))
	}

	output, cost, err := agent.ParseOutput(result.Stdout)
	if err != nil {
		return AgentResult{
			Output:    result.Stdout,
			DurationS: time.Since(start).Seconds(),
		}, err
	}

	// Save raw JSON for codex cost tracking.
	if cost > 0 {
		if runDir := strings.TrimSpace(req.RunDir); runDir != "" {
			prefix := strings.TrimSpace(req.OutputPrefix)
			if prefix == "" {
				prefix = "agent"
			}
			rawFile := filepath.Join(runDir, prefix+".raw.json")
			_ = os.WriteFile(rawFile, []byte(result.Stdout), 0o644)
		}
	}

	return AgentResult{
		Output:    output,
		CostUSD:   cost,
		DurationS: time.Since(start).Seconds(),
	}, nil
}

// EnsureSession delegates to the inner runner if it implements SessionManager.
func (r *composedRuntime) EnsureSession() error {
	if sm, ok := r.runner.(SessionManager); ok {
		return sm.EnsureSession()
	}
	return nil
}

// Cleanup delegates to the inner runner if it implements SessionManager.
func (r *composedRuntime) Cleanup() {
	if sm, ok := r.runner.(SessionManager); ok {
		sm.Cleanup()
	}
}

// SessionName delegates to the inner runner if it implements SessionManager.
func (r *composedRuntime) SessionName() string {
	if sm, ok := r.runner.(SessionManager); ok {
		return sm.SessionName()
	}
	return ""
}

// defaultAgents returns the built-in agent spec registry.
func defaultAgents() map[Agent]AgentSpec {
	return map[Agent]AgentSpec{
		AgentCodex:  &codexAgent{},
		AgentClaude: &claudeAgent{},
	}
}
