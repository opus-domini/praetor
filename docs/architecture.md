# Architecture

## Overview

`praetor` is a CLI-first orchestrator with two execution modes:

- **Plan-driven execution** (`praetor plan run <plan-file>`) with Plan/Execute/Review phases.
- **Single-prompt dispatch** (`praetor exec`) through a centralized multi-provider `Agent` abstraction.

Built-in providers:

- `codex` (CLI)
- `claude` (CLI)
- `gemini` (CLI)
- `ollama` (REST)

## Package boundaries

```text
cmd/praetor/                      CLI entrypoint
internal/
├── agents/                       Central polymorphic Agent interface + adapters (CLI/REST)
├── cli/                          Cobra command wiring (`plan`, `exec`)
├── config/                       Config loader (`config.toml`, global + per-project)
├── loop/                         Plan runtime (FSM, transitions, retries, review gates)
├── orchestrator/                 Legacy single-prompt provider engine
├── paths/                        XDG and legacy path resolution
├── runtime/
│   └── pty/                      Interactive pseudo-terminal sessions (start/read/write/close)
├── state/                        Project-local transactional snapshots and root resolvers
├── workspace/                    Git root resolution and `praetor.{yaml,yml,md}` loading
└── providers/                    Provider SDK ports (Claude/Codex)
```

## Execution flow

### `praetor exec`

1. CLI resolves provider and prompt.
2. CLI builds the default `agents.Registry` (CLI + REST adapters).
3. Selected `Agent` executes prompt through `Execute(...)`.
4. Response is printed to stdout.

### `praetor plan run <plan-file>`

1. CLI resolves plan path, workdir, and merged config.
2. Runner resolves git project root and reads workspace manifest (`praetor.yaml`, `praetor.yml`, fallback `praetor.md`).
3. Runner initializes state store (XDG roots) and acquires run lock.
4. Runner restores from latest local project snapshot when newer and compatible.
5. Optional planner phase (`--objective`) generates a plan before execution.
6. Main loop runs as FSM (`stateFn`) with explicit transitions:
   - guard/cancellation checks
   - task selection
   - execute
   - review
   - finalize
7. Each transition persists:
   - mutable plan state
   - checkpoints/metrics
   - transactional local snapshot (`.praetor/runtime/<run-id>/snapshot.json` + journal)
8. Runner exits on completion, cancellation, blockage, or iteration cap.

## Runtime model

`loop.Runner` chooses runtime by mode:

- `tmux`: classic composed runtime (`codex`/`claude`) with visible tmux windows.
- `direct` / `pty`: central `agents.Agent` registry runtime (supports CLI + REST backends).

The cognitive strategy is **Plan-and-Execute** with explicit **Review gate**:

- `Plan`: macro decomposition
- `Execute`: isolated task execution
- `Review`: independent verification

## PTY model

`internal/runtime/pty` exposes interactive sessions with first-class operations:

- `Start(ctx, spec)`
- `Events() <-chan StreamEvent`
- `Write(input)`
- `CloseInput()`
- `Wait()`
- `Close()`

This enables bidirectional interaction with CLI tools that require a real TTY.

## State and recovery

State is split into two layers:

- **XDG project state/cache**: canonical mutable execution state, checkpoints, locks, logs.
- **Local transactional snapshots**: `.praetor/runtime/<run-id>/` for crash-safe recovery.

Snapshot files:

- `snapshot.json`
- `events.log`
- `lock.json`
- `meta.json`

Writes are atomic (`tmp + rename`) and synced (`fsync`) on critical paths.

## Design principles

- Small packages with clear ownership.
- Explicit interfaces over implicit coupling.
- Provider transport isolation (CLI and REST behind one `Agent` contract).
- Workspace context anchored at repository root.
- FSM-driven orchestration to avoid nested, brittle control flow.
- Transactional recovery by default.
