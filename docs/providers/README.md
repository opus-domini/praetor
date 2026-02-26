# Providers

Praetor uses a single canonical provider abstraction: `internal/agents.Agent`.
The same contract is used for all runner modes (`tmux`, `direct`, `pty`) and all phases (`plan`, `execute`, `review`).

## Canonical contract

```go
type Agent interface {
    ID() ID
    Capabilities() Capabilities
    Plan(ctx context.Context, req PlanRequest) (PlanResponse, error)
    Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error)
    Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error)
}
```

Each response exposes `strategy` (`structured`, `process`, `pty`) so runtime behavior is auditable.

## Built-in providers

| Provider | Transport | Endpoint / Binary |
|----------|-----------|-------------------|
| [Claude](claude.md) | CLI (`stream-json`) | `claude` |
| [Codex](codex.md) | CLI (`exec --json`) | `codex` |
| Gemini | CLI | `gemini` |
| Ollama | REST | `http://127.0.0.1:11434` |

## Adding a new provider

1. Implement a new `internal/agents.Agent` adapter (CLI or REST).
2. Register it in `internal/agents.NewDefaultRegistry(...)`.
3. Add/update flags in `internal/cli/run.go` when needed.
4. Add tests covering `plan`, `execute`, and `review` contract behavior.
5. Document usage in `docs/providers/<name>.md`.
