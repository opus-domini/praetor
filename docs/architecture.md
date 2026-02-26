# Architecture

## Overview

`praetor` is a CLI-first orchestrator with two complementary execution modes:

- **Plan-driven execution** (`praetor run <plan>`) — execute a dependency-ordered sequence of tasks through an executor/reviewer pipeline inside tmux sessions.
- **Single-prompt dispatch** (`praetor exec`) — send one prompt to one provider, get the response.

Current providers: **Claude Code** and **Codex**.

## Package boundaries

```text
cmd/praetor/                      CLI entrypoint (main.go)
internal/
├── cli/                          Cobra command wiring and flag parsing
│   ├── root.go                   Root command
│   ├── run.go                    praetor run <plan-file>
│   ├── plan.go                   praetor plan {create,list,status,reset}
│   └── exec.go                   praetor exec [prompt]
├── config/                       User configuration
│   └── config.go                 ~/.praetor/config.toml loader and parser
├── loop/                         Plan-driven orchestration runtime
│   ├── types.go                  Plan, Task, State, TaskStatus, AgentRuntime, IsolationMode
│   ├── plan.go                   Plan loading, validation, checksum, scaffolding
│   ├── store.go                  Mutable state, locks, costs, checkpoints
│   ├── transition.go             State machine: transition table, validator, status normalization
│   ├── runner.go                 Main loop: dependency resolution, step-function dispatch
│   ├── runner_outcome.go         Task outcome application (retry, complete, cancel)
│   ├── runner_policy.go          TransitionRecorder, IsolationPolicy (git worktree)
│   ├── agents.go                 SDKAgentRuntime (in-process Claude/Codex SDK calls)
│   ├── agents_tmux.go            TMUXAgentRuntime (tmux-window subprocess execution)
│   ├── prompts.go                Executor/reviewer prompt builders, git diff capture
│   ├── output.go                 Colored terminal renderer
│   └── state.go                  Dependency graph evaluation, blocked task detection
├── orchestrator/                 Provider contract and dispatch engine
│   ├── orchestrator.go           Engine, Registry, Request, Response types
│   └── provider.go               Provider interface definition
└── providers/
    ├── claude/                   Claude Code SDK port
    │   ├── types.go              SDKMessage, ResultMessage, AssistantMessage, etc.
    │   ├── options.go            Options, PermissionUpdate, SandboxSettings, hooks
    │   ├── query.go              Query lifecycle, message handling, parse helpers
    │   ├── prompt.go             One-shot Prompt() helper
    │   ├── sessions.go           ListSessions for persisted local sessions
    │   ├── transport_process.go  Subprocess transport (stdin/stdout stream-json)
    │   └── adapter.go            Orchestrator adapter (Provider interface)
    └── codex/                    Codex SDK port
        ├── types.go              Stream items, thread options, turn results
        ├── codex.go              Client, thread management, turn execution
        └── adapter.go            Orchestrator adapter (Provider interface)
```

### Package responsibilities

| Package | Responsibility |
|---------|---------------|
| `cmd/praetor` | Process entrypoint. Calls `cli.NewRootCmd().Execute()`. |
| `internal/cli` | Cobra command tree (`run`, `plan`, `exec`), flag parsing, config loading, provider construction. No business logic. |
| `internal/config` | User configuration loader. Reads `~/.praetor/config.toml` with global defaults and per-project overrides. Zero external dependencies. |
| `internal/loop` | Immutable plan model, mutable state store, task state machine (5-state FSM with validated transitions), dependency graph, runner pipeline, agent runtimes, prompt construction, terminal output. |
| `internal/orchestrator` | Provider contract (`Provider` interface), request/response types, provider registry, dispatch engine. |
| `internal/providers/claude` | Full Go port of `@anthropic-ai/claude-agent-sdk`. Communicates with the `claude` CLI process over `stream-json`. |
| `internal/providers/codex` | Full Go port of `@openai/codex-sdk`. Communicates with the `codex` CLI process over JSONL. |

## Execution flow

### Single-prompt mode (`praetor exec`)

1. CLI parses `--provider` and the prompt argument (or reads stdin).
2. `buildProvider()` constructs the selected provider implementation.
3. Provider is registered in the orchestration registry.
4. Orchestration engine validates the request and dispatches to the provider.
5. Provider adapter translates between the orchestrator contract and the provider-specific SDK surface.
6. Response is printed to stdout.

### Plan execution (`praetor run <plan>`)

1. CLI loads and validates the plan JSON file (immutable input).
2. Runner acquires a PID-based lock to prevent concurrent runs of the same plan.
3. State store initializes or merges mutable state, detecting plan changes via SHA-256 checksum. Legacy statuses and transient states are normalized on load (crash recovery).
4. Pre-flight validation checks that required binaries (`claude`, `codex`, `tmux`) are in PATH.
5. Runner selects the next runnable task (status = `pending`, all dependencies `done`).
6. **Retry guard**: if `task.Attempt >= maxRetries`, the task transitions to `failed` (terminal).
7. Task transitions to `executing` (persisted before the agent runs).
8. **Worktree isolation**: a dedicated `git worktree` is created for the task (when `--isolation worktree`).
9. **Executor phase**: agent runs the task prompt in the worktree via a tmux window, producing output and cost data.
10. **Post-task hook** (optional): custom script runs for validation (linters, tests). Failure triggers retry with feedback.
11. Task transitions to `reviewing` (persisted before the reviewer runs).
12. **Reviewer phase**: independent agent evaluates executor output + git diff, returns `PASS|reason` or `FAIL|reason`.
13. On pass: task transitions to `done`, worktree branch is merged into main and cleaned up.
14. On fail: `task.Attempt` increments, `task.Feedback` stores the reason, task transitions back to `pending`, worktree is deleted without merging.
15. **Cost tracking**: per-invocation costs are recorded in a TSV ledger.
16. **Checkpoint audit**: every state transition is logged to `checkpoints/history.tsv`.
17. Loop exits when all tasks reach terminal states (`done`/`failed`), dependencies block progress, or iteration limits are reached.
18. Summary reports total tasks done, rejected, iterations, accumulated cost, and duration.

## Agent runtimes

The `AgentRuntime` interface decouples task execution from the transport mechanism:

```go
type AgentRuntime interface {
    Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}
```

Two implementations:

| Runtime | How it works |
|---------|-------------|
| `TMUXAgentRuntime` | Creates a wrapper shell script, launches it in a tmux window, uses `tmux wait-for` to block until completion. Captures stdout/stderr/exit code as files. Extracts cost from Codex JSON output. Default runtime for `praetor run`. |
| `SDKAgentRuntime` | Calls the Claude/Codex Go SDK ports directly in-process. Used by `praetor exec` (single-prompt mode). |

## Dependencies

One external dependency: [`cobra`](https://github.com/spf13/cobra) for CLI parsing. Everything else is Go standard library.

## Design rationale

- **Packages are small and focused.** Each package owns one concept and exposes a minimal surface.
- **Explicit dependencies over global state.** No `init()` functions, no package-level mutable state.
- **Provider isolation.** Provider-specific logic never leaks outside `internal/providers/`. The orchestrator and loop packages interact only through the `Provider` and `AgentRuntime` interfaces.
- **Immutable plans, mutable state.** Plan files are never modified at runtime. All mutable data lives under `~/.praetor/projects/<project-hash>/` and can be safely deleted or reset.
- **Observable execution.** Every agent step runs in a visible tmux window. Prompts, outputs, and scripts are persisted as files for debugging.
