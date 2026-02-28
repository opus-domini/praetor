# Architecture

## Overview

`praetor` is a CLI-first orchestrator with two primary execution paths:

- **Plan orchestration**: `praetor plan run <slug>`
- **Single dispatch**: `praetor exec`

Providers supported by the unified agent abstraction:

- `claude` (CLI)
- `codex` (CLI)
- `copilot` (CLI)
- `gemini` (CLI)
- `kimi` (CLI)
- `opencode` (CLI)
- `openrouter` (REST)
- `ollama` (REST)

## Package boundaries

```text
cmd/praetor/                      CLI entrypoint
internal/
├── agent/                        Provider abstraction + adapters + middleware
│   ├── adapters/                 CLI and REST implementations
│   ├── middleware/               Logging, metrics, event sinks
│   ├── runner/                   Command execution abstraction
│   ├── runtime/                  Registry + fallback runtime
│   └── text/                     Prompt/output helpers
├── app/                          Bootstrap and dependency wiring
├── cli/                          Cobra commands and renderer
├── config/                       Config loading and normalization
├── domain/                       Core types, parsing, transitions, validation
├── orchestration/
│   ├── fsm/                      Generic state-function engine
│   └── pipeline/                 Plan/Execute/Review loop + cognitive agents
├── prompt/                       Template engine (embedded + overlay)
├── runtime/                      Process, PTY, tmux execution backends
├── state/                        Stores, locks, checkpoints, snapshots
└── workspace/                    Project root/manifest resolution
```

## Domain model

`internal/domain` is dependency-free and centralizes:

- Plan schema v1 (`Plan`, `Task`, `PlanSettings`, `PlanQuality`, `ExecutionPolicy`)
- Mutable run state (`State`, `StateTask`, `TaskStatus`)
- Runtime config (`RunnerOptions`)
- Parsing contracts (`ParseExecutorResult`, `ParseReviewDecision`, `ParseGateEvidence`)
- Transitions/graph (`Transition`, `NextRunnableTask`, blocked-dependency reporting)

Plan loading is strict and two-pass:

1. Detect known legacy fields and return migration-oriented errors.
2. Strict decode with `DisallowUnknownFields()`.

## Execution flows

### `praetor exec`

1. CLI resolves provider and prompt.
2. Runtime registry resolves adapter.
3. Agent executes.
4. Output is written to stdout.

### `praetor plan run <slug>`

1. Resolve project root and workspace manifest.
2. Load plan and state store.
3. Merge runtime options with precedence (`CLI > plan.settings > resolved config file > defaults`).
4. Build runtime stack with shared event sink.
5. Run loop FSM:
   - select runnable task
   - execute
   - review/gates
   - apply outcome
   - persist snapshots/events/metrics
6. Finalize run with explicit `RunOutcome` and exit code.

## Runtime composition

The runtime is assembled as decorators:

```text
RegistryRuntime
  └── FallbackRuntime
       └── Logging middleware
            └── Metrics middleware
```

`FallbackRuntime` uses error classification (`transient`, `auth`, `rate_limit`, `unsupported`, `unknown`) plus configured fallback mappings.

## Prompt system

`internal/prompt` loads templates in two layers:

1. Embedded defaults (`go:embed`)
2. Optional project overlay (`.praetor/prompts/*.tmpl`)

Available templates:

| Template | Purpose |
|---|---|
| `executor.system.tmpl` | executor role/system instructions |
| `executor.task.tmpl` | task payload, retries, acceptance, required gates |
| `reviewer.system.tmpl` | reviewer role/system instructions |
| `reviewer.task.tmpl` | task payload, executor output, git diff |
| `planner.system.tmpl` | planner schema instructions (plan v1) |
| `planner.task.tmpl` | objective/brief payload |
| `adapter.plan.tmpl` | provider-shared planning prompt |
| `adapter.plan.claude.tmpl` | Claude-specific planning prompt |

## Observability and diagnostics

Structured runtime diagnostics are persisted per run:

- `runtime/<run-id>/events.jsonl`
- `runtime/<run-id>/diagnostics/performance.jsonl`
- `runtime/<run-id>/snapshot.json`

Emitted events include:

- `agent_start`, `agent_complete`, `agent_error`, `agent_fallback`
- `task_stalled`
- `budget_warning`
- `gate_result`

This stream is consumed by `praetor plan diagnose`.

## State and recovery

Project data is isolated under `<praetor-home>/projects/<project-key>/`:

- `plans/` plan files
- `state/` mutable state
- `locks/` run locks
- `checkpoints/` transition ledger
- `costs/` cost metrics
- `logs/` per-invocation logs
- `runtime/<run-id>/` transactional snapshots + diagnostics

Recovery behavior:

- latest valid snapshot may restore newer state
- checksum mismatch prevents stale/foreign restore
- transient in-progress states are reset to `pending` on load

## Intelligent routing

Agent availability is probed at bootstrap. Executor routing uses plan-level defaults and availability:

1. Use configured default executor when healthy.
2. Otherwise auto-select from available executors (CLI preferred over REST).

There is no per-task agent override in plan schema v1.

## Design principles

- CLI-first operational UX
- Strict schemas and explicit failure modes
- Filesystem as auditable source of truth
- Context-budgeted prompts for predictable execution
- Event-driven diagnostics without mandatory UI/dashboard
