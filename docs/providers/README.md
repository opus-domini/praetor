# Providers

Praetor abstracts AI agent backends behind two layers:

1. **Orchestrator contract** (`internal/orchestrator`) — the `Provider` interface used by `praetor run` for single-prompt dispatch.
2. **Agent runtime** (`internal/loop`) — the `AgentRuntime` interface used by `praetor loop run` for plan-driven execution.

Both providers implement the orchestrator contract. The loop runner drives agents either through the SDK runtimes directly (`SDKAgentRuntime`) or through tmux subprocess execution (`TMUXAgentRuntime`).

## Current providers

| Provider | Package | Transport | Binary |
|----------|---------|-----------|--------|
| [Claude](claude.md) | `internal/providers/claude` | `stream-json` over stdin/stdout | `claude` |
| [Codex](codex.md) | `internal/providers/codex` | JSONL over stdin/stdout | `codex` |

## Provider interface

The orchestrator contract:

```go
type Provider interface {
    ID() ProviderID
    Run(ctx context.Context, req Request) (Response, error)
}
```

The loop agent runtime contract:

```go
type AgentRuntime interface {
    Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}
```

`AgentResult` carries the output text, cost in USD, and duration in seconds.

## Adding a new provider

1. Create a package under `internal/providers/<name>/`.
2. Implement the subprocess transport (stdin/stdout protocol).
3. Implement `orchestrator.Provider` for single-prompt mode.
4. Register it in `internal/cli/root.go` (`knownProviders()` and `buildProvider()`).
5. Add an agent constant in `internal/loop/types.go` and update `validExecutors`/`validReviewers`.
6. Update `SDKAgentRuntime.Run()` in `internal/loop/agents.go` with a new case.
7. Update `buildWrapperScript()` in `internal/loop/agents_tmux.go` for tmux execution.
8. Add documentation in `docs/providers/<name>.md`.
