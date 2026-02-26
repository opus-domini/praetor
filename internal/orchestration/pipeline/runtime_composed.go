package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agents"
	"github.com/opus-domini/praetor/internal/domain"
	claudeprovider "github.com/opus-domini/praetor/internal/providers/claude"
	codexprovider "github.com/opus-domini/praetor/internal/providers/codex"
	processruntime "github.com/opus-domini/praetor/internal/runtime/process"
	ptyruntime "github.com/opus-domini/praetor/internal/runtime/pty"
	tmuxruntime "github.com/opus-domini/praetor/internal/runtime/tmux"
)

// composedRuntime implements domain.AgentRuntime by composing AgentSpec and ProcessRunner.
type composedRuntime struct {
	agents map[domain.Agent]domain.AgentSpec
	runner domain.ProcessRunner
}

func newComposedRuntime(agents map[domain.Agent]domain.AgentSpec, runner domain.ProcessRunner) *composedRuntime {
	return &composedRuntime{agents: agents, runner: runner}
}

func (r *composedRuntime) Run(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	start := time.Now()

	agent, ok := r.agents[domain.NormalizeAgent(req.Agent)]
	if !ok {
		return domain.AgentResult{}, fmt.Errorf("unsupported agent %q", req.Agent)
	}

	spec, err := agent.BuildCommand(req)
	if err != nil {
		return domain.AgentResult{DurationS: time.Since(start).Seconds()}, err
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
		return domain.AgentResult{DurationS: time.Since(start).Seconds()}, err
	}

	if result.ExitCode != 0 {
		return domain.AgentResult{
				Output:    result.Stdout,
				DurationS: time.Since(start).Seconds(),
			}, fmt.Errorf("agent process failed with exit code %d: %s",
				result.ExitCode, tmuxruntime.TailText(result.Stderr, 20))
	}

	output, cost, err := agent.ParseOutput(result.Stdout)
	if err != nil {
		return domain.AgentResult{
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

	return domain.AgentResult{
		Output:    output,
		CostUSD:   cost,
		DurationS: time.Since(start).Seconds(),
	}, nil
}

// EnsureSession delegates to the inner runner if it implements SessionManager.
func (r *composedRuntime) EnsureSession() error {
	if sm, ok := r.runner.(domain.SessionManager); ok {
		return sm.EnsureSession()
	}
	return nil
}

// Cleanup delegates to the inner runner if it implements SessionManager.
func (r *composedRuntime) Cleanup() {
	if sm, ok := r.runner.(domain.SessionManager); ok {
		sm.Cleanup()
	}
}

// SessionName delegates to the inner runner if it implements SessionManager.
func (r *composedRuntime) SessionName() string {
	if sm, ok := r.runner.(domain.SessionManager); ok {
		return sm.SessionName()
	}
	return ""
}

// defaultAgents returns the built-in agent spec registry.
func defaultAgents() map[domain.Agent]domain.AgentSpec {
	return map[domain.Agent]domain.AgentSpec{
		domain.AgentCodex:  &codexprovider.AgentSpec{},
		domain.AgentClaude: &claudeprovider.AgentSpec{},
	}
}

// buildProcessRunner creates a ProcessRunner for the given runner mode.
func buildProcessRunner(opts domain.RunnerOptions) (domain.ProcessRunner, error) {
	switch opts.RunnerMode {
	case domain.RunnerTMUX:
		return tmuxruntime.NewRunner(opts.TMUXSession), nil
	case domain.RunnerPTY:
		return &ptyruntime.Runner{}, nil
	case domain.RunnerDirect:
		return &processruntime.Runner{}, nil
	default:
		return nil, fmt.Errorf("unsupported runner mode %q", opts.RunnerMode)
	}
}

// BuildAgentRuntime creates an AgentRuntime for the given runner options.
func BuildAgentRuntime(opts domain.RunnerOptions) (domain.AgentRuntime, error) {
	switch opts.RunnerMode {
	case domain.RunnerTMUX:
		processRunner, err := buildProcessRunner(opts)
		if err != nil {
			return nil, err
		}
		return newComposedRuntime(defaultAgents(), processRunner), nil
	case domain.RunnerPTY, domain.RunnerDirect:
		return agents.NewRegistryRuntime(opts), nil
	default:
		return nil, fmt.Errorf("unsupported runner mode %q", opts.RunnerMode)
	}
}
