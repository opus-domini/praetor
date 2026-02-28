# Providers

Praetor abstracts all AI agents behind a unified `agent.Agent` interface. Each provider is implemented as an adapter in `internal/agent/adapters/` and registered in a shared registry at startup. The same contract is used for all runner modes and all pipeline phases.

## Agent interface

```go
type Agent interface {
    ID() ID
    Capabilities() Capabilities
    Plan(ctx context.Context, req PlanRequest) (PlanResponse, error)
    Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error)
    Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error)
}
```

All providers implement all three operations. Adapters that don't natively distinguish plan/execute/review internally route to the same underlying invocation logic and rely on prompt content to drive behavior.

## Capabilities

Each adapter declares its capabilities at registration:

```go
type Capabilities struct {
    Transport        Transport  // "cli" or "rest"
    SupportsPlan     bool
    SupportsExecute  bool
    SupportsReview   bool
    RequiresTTY      bool       // adapter needs a PTY for streaming
    StructuredOutput bool       // adapter produces parseable JSON output
}
```

`RequiresTTY` triggers the PTY execution path (via the `script` utility). `StructuredOutput` indicates whether the adapter returns JSON that can be parsed for cost, model, and structured content.

## Built-in providers

| Provider | Transport | Binary / Endpoint | TTY | Structured | Install |
|---|---|---|---|---|---|
| [Claude](claude.md) | CLI | `claude` | yes | yes | `npm i -g @anthropic-ai/claude-code` |
| [Codex](codex.md) | CLI | `codex` | no | yes | `npm i -g @openai/codex` |
| [Copilot](copilot.md) | CLI | `copilot` | no | no | `npm i -g @githubnext/github-copilot-cli` |
| [Gemini](gemini.md) | CLI | `gemini` | yes | no | `npm i -g @google/gemini-cli` |
| [Kimi](kimi.md) | CLI | `kimi` | yes | no | See https://kimi.ai |
| [OpenCode](opencode.md) | CLI | `opencode` | no | no | `go install github.com/opencode-ai/opencode@latest` |
| [OpenRouter](openrouter.md) | REST | `https://openrouter.ai/api/v1` | no | yes | API key via `OPENROUTER_API_KEY` |
| [Ollama](ollama.md) | REST | `http://127.0.0.1:11434` | no | no | https://ollama.com |

## Runner abstraction

Adapters don't launch processes directly. They build a `CommandSpec` and delegate to a `CommandRunner`:

```go
type CommandSpec struct {
    Args         []string
    Env          []string
    Dir          string   // working directory
    Stdin        string   // content piped to stdin (empty = none)
    UsePTY       bool     // use script-based PTY
    RunDir       string   // directory for stdout/stderr capture
    OutputPrefix string   // file prefix for output files
    WindowHint   string   // tmux window name
}
```

The `CommandRunner` implementation varies by runner mode:

| Runner mode | Implementation | Behavior |
|---|---|---|
| _(default)_ | `ExecCommandRunner` | Process exec; auto-fallback to PTY on TTY errors |
| `direct` | `ExecCommandRunner{DisablePTY}` | Process exec only, no PTY fallback |
| `pty` | `ExecCommandRunner{ForcePTY}` | Always use PTY via `script` utility |
| `tmux` | `ProcessRunner` adapter | Each invocation runs in a dedicated tmux window |

The PTY runtime uses the POSIX `script` utility, auto-detecting GNU vs BSD variants at startup.

## Registry

`agent.Registry` is a thread-safe `map[ID]Agent` populated at bootstrap by `runtime.NewDefaultRegistry()`:

```go
registry.Register(adapters.NewCodexCLI(codexBin, runner))
registry.Register(adapters.NewClaudeCLI(claudeBin, runner))
registry.Register(adapters.NewCopilotCLI(copilotBin, runner))
registry.Register(adapters.NewGeminiCLI(geminiBin, runner))
registry.Register(adapters.NewKimiCLI(kimiBin, runner))
registry.Register(adapters.NewOpenCodeCLI(opencodeBin, runner))
registry.Register(adapters.NewOpenRouterREST(url, model, keyEnv, httpClient))
registry.Register(adapters.NewOllamaREST(url, model, httpClient))
```

Binary paths, URLs, and models are all configurable via CLI flags or config file.

## Runtime composition

The runtime is assembled as a decorator stack:

```
middleware.Chain(fallbackRuntime, Logging, Metrics)
  └── FallbackRuntime
       └── RegistryRuntime
            └── Registry → Adapter → CommandRunner
```

`RegistryRuntime` routes `AgentRequest` to the correct adapter based on the `Agent` field, then dispatches by role:

- `"plan"` → `adapter.Plan()`
- `"review"` → `adapter.Review()`
- anything else → `adapter.Execute()`

## Fallback runtime

`FallbackRuntime` wraps `RegistryRuntime` and intercepts errors:

1. Classify the error via pattern matching on the error message.
2. Resolve a fallback agent via `FallbackPolicy`.
3. Retry with the fallback agent.
4. Emit an `agent_fallback` event.

Error classification:

| Class | Patterns |
|---|---|
| `rate_limit` | 429, "rate limit", "too many requests" |
| `auth` | 401, 403, "api key", "unauthorized", "forbidden" |
| `transient` | "connection refused", "timeout", 502/503/504, "broken pipe", "eof" |
| `unsupported` | "unsupported", "not implemented" |
| `unknown` | anything else |

Fallback policy (from config):

| Config key | Behavior |
|---|---|
| `fallback` | Per-agent mapping: primary -> fallback |
| `fallback-on-transient` | Global fallback for transient errors |
| `fallback-on-auth` | Global fallback for auth errors |

## Middleware pipeline

Middleware follows the standard wrapping pattern:

```go
type Middleware func(next domain.AgentRuntime) domain.AgentRuntime
```

Built-in middleware:

| Middleware | Purpose |
|---|---|
| **Logging** | Emits `agent_start`, `agent_complete`, `agent_error` events; writes structured log entries |
| **Metrics** | Records per-`(agent, role, status)` invocation counts and total cost |

Events are written to a configurable sink (JSONL file, multiplexed, or in-memory for tests).

## Health probing

`agent.Prober` checks agent availability at startup (used by `praetor doctor` and intelligent routing):

- **CLI agents**: resolves binary via `exec.LookPath`, runs `<binary> --version`, parses semver from stdout/stderr (strips ANSI codes).
- **REST agents**: sends `GET {baseURL}{healthEndpoint}`, checks for HTTP 2xx/3xx.

Status values: `ok`, `not_found`, `error`, `unreachable`.

## Prompt system

Adapters receive prompts from the pipeline's template engine. Two adapter-specific templates exist:

| Template | Used by | Purpose |
|---|---|---|
| `adapter.plan.tmpl` | All adapters (except Claude) | Plan prompt: workspace context + objective |
| `adapter.plan.claude.tmpl` | Claude only | Plan system prompt for Claude's `--append-system-prompt` |

Templates are embedded via `go:embed` with optional project overlay from `.praetor/prompts/`.

## Common utilities

Shared helpers in `adapters/common.go`:

| Function | Purpose |
|---|---|
| `ComposePrompt(system, prompt)` | Concatenates system + user prompt with `\n\n` separator |
| `ExtractJSONObject(input)` | Finds first `{` to last `}` — used by `Plan()` to extract JSON manifests |
| `ParseReview(output)` | Scans for `DECISION: PASS/FAIL` and `REASON:` lines |
| `TailText(input, maxLines)` | Last N lines joined with ` \| ` — used for error messages |

## Adding a new provider

1. Create an adapter in `internal/agent/adapters/` implementing `agent.Agent`.
2. Add a catalog entry in `internal/agent/catalog.go` with metadata (display name, transport, binary, install instructions, health endpoint).
3. Register the adapter in `internal/agent/runtime/defaults.go`.
4. Add binary/URL config keys to `internal/config/parser.go` (`allowedKeys`), `internal/config/defaults.go` (`Registry`), and `internal/config/config.go` (`Config` struct + `configFromMap` + `mergeConfig`).
5. Add CLI flags in `internal/cli/run.go`.
6. Write tests covering `Plan()`, `Execute()`, and `Review()` contract behavior.
7. Document in `docs/providers/<name>.md`.
