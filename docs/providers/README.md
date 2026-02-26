# Providers

Praetor abstracts AI agent backends behind two layers:

1. **Agent contract** (`internal/domain`) — the `AgentSpec` interface for subprocess command generation (used by tmux runtime) and the `AgentRuntime` interface for plan-driven execution.
2. **Agent registry** (`internal/agents`) — polymorphic `Agent` interface with CLI and REST adapters for single-prompt dispatch (`praetor exec`).

Both layers share the domain types from `internal/domain`. The pipeline runner (`internal/orchestration/pipeline`) drives agents either through the tmux composed runtime or through the agent registry runtime (direct/pty modes).

## Current providers

| Provider | Package | Transport | Binary/Endpoint |
|----------|---------|-----------|-----------------|
| [Claude](claude.md) | `internal/providers/claude` | `stream-json` over stdin/stdout | `claude` |
| [Codex](codex.md) | `internal/providers/codex` | JSONL over stdin/stdout | `codex` |
| Gemini | `internal/providers/gemini` | CLI subprocess | `gemini` |
| Ollama | `internal/providers/ollama` | REST API | `http://127.0.0.1:11434` |

## Provider interfaces

The agent spec contract (for tmux-mode command generation):

```go
type AgentSpec interface {
    BuildCommand(req AgentRequest) CommandSpec
    ParseResult(stdout, stderr string, exitCode int) AgentResult
}
```

The agent runtime contract (for direct/pty-mode execution):

```go
type AgentRuntime interface {
    Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}
```

`AgentResult` carries the output text, cost in USD, and duration in seconds.

## Adding a new provider

1. Create a package under `internal/providers/<name>/`.
2. Implement the subprocess transport (stdin/stdout protocol or REST client).
3. Implement `domain.AgentSpec` for tmux-mode command generation in `<name>/spec.go`.
4. Register it in `internal/agents/` with a CLI or REST adapter.
5. Add an agent constant in `internal/domain/types.go` and update `validExecutors`/`validReviewers` in `internal/domain/plan.go`.
6. Add the agent to `defaultAgents()` in `internal/orchestration/pipeline/runtime_composed.go`.
7. Add CLI flags (e.g. `--<name>-bin`) in `internal/cli/run.go`.
8. Add documentation in `docs/providers/<name>.md`.
