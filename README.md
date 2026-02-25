<div align="center">
    <img src="docs/assets/images/logo.png" alt="Logo Praetor" width="500"/>
    <hr />
    <p>Lead. Delegate. Dominate.</p>
</div>

`praetor` is a Go CLI that orchestrates AI agent providers through a single execution surface. It drives Claude Code and Codex as subprocess agents, coordinated by an executor/reviewer loop that runs tasks inside tmux sessions with full git safety, cost tracking, and crash recovery.

## Features

- **Two execution modes** — single-prompt dispatch (`praetor run`) and plan-driven orchestration (`praetor loop run`).
- **Provider abstraction** — Claude and Codex behind a common interface. Add new providers without changing CLI or loop logic.
- **Tmux-first execution** — every agent invocation runs in a dedicated tmux window for live operational visibility.
- **Executor/reviewer pipeline** — each task runs an executor agent, then an independent reviewer agent that gates promotion.
- **Git safety** — automatic `HEAD` snapshot before each task, rollback on failure, discard on success.
- **Cost tracking** — per-invocation cost ledger (TSV), Claude `total_cost_usd` and Codex JSON cost extraction, summary reporting.
- **Crash recovery** — PID-locked runs, SHA-256 plan checksums, mutable state files, retry counters, and feedback persistence.
- **Checkpoint audit log** — append-only history of every state transition for post-mortem analysis.
- **Post-task hooks** — run custom validation (linters, tests) between executor and reviewer phases.
- **Dependency graph** — tasks declare `depends_on` edges; the runner selects the next task whose dependencies are satisfied.

## Repository layout

```text
.
├── cmd/praetor/                  # CLI entrypoint
├── internal/
│   ├── cli/                      # Cobra command wiring
│   ├── loop/                     # Plan, state, runner, agents, prompts, output
│   ├── orchestrator/             # Provider contract, registry, dispatch engine
│   └── providers/
│       ├── claude/               # Claude Code SDK port (stream-json subprocess)
│       └── codex/                # Codex SDK port (JSONL subprocess)
├── docs/                         # Project documentation (docsify)
│   ├── schemas/                  # JSON Schema for plan files
│   └── plans/                    # Plan files for loop execution
└── .rfcs/                        # Design RFCs
```

## Quick start

Prerequisites: Go 1.26, `tmux`, and at least one agent binary (`claude` or `codex`) in `PATH`.

Build the CLI:

```bash
go build -o build/praetor ./cmd/praetor
```

### Single-prompt mode

```bash
# Codex
./build/praetor run --provider codex --prompt "Reply with OK"

# Claude (reads from stdin)
echo "Reply with OK" | ./build/praetor run --provider claude
```

### Plan-driven loop

Create a plan skeleton:

```bash
./build/praetor loop plan new my-feature
```

Edit the generated plan at `docs/plans/PLAN-PRAETOR-<date>-my-feature.json`, then run it:

```bash
./build/praetor loop run --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

Check progress:

```bash
./build/praetor loop plan status --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

List all tracked plans:

```bash
./build/praetor loop plan list
```

Reset execution state for a plan:

```bash
./build/praetor loop plan reset --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

## CLI reference

### `praetor run`

Run a single prompt on a provider.

| Flag | Default | Description |
|------|---------|-------------|
| `--provider` | `codex` | Provider: `codex` or `claude` |
| `--prompt` | stdin | Prompt text (reads stdin if empty) |
| `--timeout` | none | Timeout duration (e.g. `30s`, `5m`) |

### `praetor loop run`

Run a task plan with executor/reviewer orchestration.

| Flag | Default | Description |
|------|---------|-------------|
| `--plan` | required | Plan JSON file path |
| `--default-executor` | `codex` | Default executor: `codex` or `claude` |
| `--default-reviewer` | `claude` | Default reviewer: `codex`, `claude`, or `none` |
| `--max-retries` | `3` | Maximum retries per task |
| `--max-iterations` | `0` | Maximum loop iterations (0 = unlimited) |
| `--skip-review` | `false` | Auto-approve executor outputs |
| `--git-safety` | `true` | Enable git snapshot/rollback on failure |
| `--post-task-hook` | none | Script to run after executor, before reviewer |
| `--codex-bin` | `codex` | Codex binary path |
| `--claude-bin` | `claude` | Claude binary path |
| `--tmux-session` | auto | tmux session name (default: `praetor-<project-hash>`) |
| `--workdir` | `.` | Working directory for agents |
| `--state-root` | auto | State root (default: `~/.praetor/projects/<project-hash>`) |
| `--force` | `false` | Override existing plan lock |
| `--no-color` | `false` | Disable colored output |
| `--timeout` | none | Timeout duration (e.g. `30m`, `2h`) |

### `praetor loop plan`

Manage plan files and execution state.

| Subcommand | Description |
|------------|-------------|
| `new <slug>` | Create a plan skeleton in `docs/plans/` |
| `status --plan <path>` | Show execution status for one plan |
| `list` | List all plans with execution state |
| `reset --plan <path>` | Clear execution state for one plan |

### `praetor providers`

List available providers.

## State layout

Runtime state is isolated per git project under `~/.praetor/projects/<project-hash>/`:

```text
~/.praetor/projects/<hash>/
├── state/          # Mutable task state per plan (.state.json)
├── locks/          # PID-based run locks per plan
├── logs/           # Per-run execution logs (prompts, outputs, scripts)
├── retries/        # Retry counters per task signature
├── feedback/       # Reviewer feedback per task signature
├── snapshots/      # Git HEAD snapshots for rollback
├── costs/          # Cost tracking ledger (tracking.tsv)
└── checkpoints/    # Audit log (history.tsv) and current checkpoint
```

## Development

```bash
make fmt          # Format code
make lint         # Run golangci-lint
make test         # Run tests
make ci           # Full local CI
```

## Documentation

Full documentation is served with [docsify](https://docsify.js.org/) from `docs/`:

- [Architecture](docs/architecture.md) — package boundaries and execution flow
- [Loop orchestration](docs/loop.md) — plan format, runtime model, and safety mechanisms
- [Providers](docs/providers/) — Claude and Codex provider details

## Design principles

- Keep packages small and focused.
- Prefer explicit dependencies over global state.
- Keep provider-specific logic isolated behind a common interface.
- Keep plan files immutable and execution state mutable and isolated.
- Keep agent execution observable in tmux sessions.
- One external dependency (`cobra`). Everything else is stdlib.
