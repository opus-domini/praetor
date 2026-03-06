<div align="center">
    <img src="docs/assets/images/logo.svg" alt="Praetor logo" width="500"/>
    <hr />
    <p><strong>Lead. Delegate. Dominate.</strong></p>
    <p>
        <a href="https://goreportcard.com/report/github.com/opus-domini/praetor"><img src="https://goreportcard.com/badge/github.com/opus-domini/praetor" alt="Go Report Badge"></a>
        <a href="https://pkg.go.dev/github.com/opus-domini/praetor"><img src="https://pkg.go.dev/badge/github.com/opus-domini/praetor.svg" alt="Go Package Docs Badge"></a>
        <a href="https://github.com/opus-domini/praetor/actions/workflows/ci.yml"><img src="https://github.com/opus-domini/praetor/actions/workflows/ci.yml/badge.svg" alt="CI Badge"></a>
        <a href="https://github.com/opus-domini/praetor/releases"><img src="https://img.shields.io/github/v/release/opus-domini/praetor" alt="Release Badge"></a>
    </p>
</div>

Praetor is a Go CLI for agent orchestration with a strict Plan-and-Execute runtime.
It orchestrates 9 AI agent providers through a single execution surface with dependency-aware plans, isolated worktrees, independent review gates, snapshot-based recovery, and structured observability.

<p align="center">
  <a href="https://opus-domini.github.io/praetor/">Documentation</a> •
  <a href="https://github.com/opus-domini/praetor/releases">Releases</a> •
  <a href="#quick-start">Quick Start</a>
</p>

## Why Praetor

- One CLI surface for planning, execution, review, and recovery.
- Unified provider abstraction across 9 CLI and REST agents.
- Explicit finite-state orchestration with transition guard rails.
- Worktree-first isolation to protect the main branch during task execution.
- Automatic fallback with error-classified failover to alternate agents.
- Middleware pipeline with composable logging and metrics.
- Structured observability via JSONL event stream and diagnostics.
- Local transactional snapshots with checksum validation and explicit resume.

## Core Capabilities

- **Plan execution** — run JSON plans with dependencies via `praetor plan run`.
- **Plan templates** — bootstrap plans from project, global, or builtin templates with `praetor plan create --from-template`, then package them with `praetor plan export`.
- **Agents** — 9 built-in providers: `claude`, `codex`, `copilot`, `gemini`, `kimi`, `lmstudio`, `opencode`, `openrouter`, and `ollama`.
- **Plan-and-Execute** — optional planner phase (`--objective`) followed by execute/review gates.
- **FSM runtime** — loop modeled as explicit states with `max-iterations` and `max-transitions` guard rails.
- **Runner modes** — `tmux`, `direct`, and `pty` under a unified runtime contract.
- **Fallback engine** — error-classified failover: per-agent mapping, transient, and auth fallback modes.
- **Stall detection** — sliding-window similarity detection with escalation (fallback agent → budget reduction → fail).
- **Prompt budget** — character-level prompt truncation for executor and reviewer phases.
- **Cost governance** — plan/task USD budgets with warnings, summaries, and optional hard enforcement.
- **Parallel waves** — dependency-aware concurrent task execution with deterministic merge ordering and conflict requeue.
- **Intelligent routing** — live health-probe-based auto-selection when the preferred agent is unavailable.
- **Quality gates** — required and optional gate enforcement in plan execution.
- **Host-executed gates** — `tests`, `lint`, `standards` run locally with structured results and diagnostics.
- **Operational evals** — local flow analysis via `praetor plan eval` (plan level) and `praetor eval` (project level).
- **Workspace context** — automatic manifest discovery from `praetor.yaml` / `praetor.md`.
- **Prompt templates** — 8 embedded templates with project-level overlay via `.praetor/prompts/`.
- **Structured feedback** — reviewer and gate feedback persisted as JSONL and reinjected into retry prompts.
- **Post-task hooks** — arbitrary script execution after executor, before reviewer (`--hook`).
- **Recovery** — automatic snapshot inspection plus manual `praetor plan resume`.
- **Retention** — local runtime pruning with `--keep-last-runs`.
- **Observability** — JSONL event stream, performance diagnostics, checkpoint history, cost ledger, actor summaries, and per-task logs.
- **Health checks** — `praetor doctor` returns structured checks, binary/endpoint paths, parsed versions, and remediation hints.
- **Configuration** — persistent config with `praetor config` (show, set, path, edit, init).

## Providers

| Provider | Transport | TTY | Structured Output |
|---|---|---|---|
| Claude | CLI | yes | yes |
| Codex | CLI | no | yes |
| Copilot | CLI | no | no |
| Gemini | CLI | yes | no |
| Kimi | CLI | yes | no |
| OpenCode | CLI | no | no |
| OpenRouter | REST | no | yes |
| LM Studio | REST | no | yes |
| Ollama | REST | no | no |

## Requirements

- Linux or macOS.
- Go 1.26+.
- `git` available in `PATH`.
- For `--runner tmux`: `tmux` installed.
- CLI agent binaries as needed: `codex`, `claude`, `copilot`, `gemini`, `kimi`, `opencode`.
- For OpenRouter: `OPENROUTER_API_KEY` env var set.
- For LM Studio: reachable REST endpoint (default `http://localhost:1234`).
- For Ollama: reachable REST endpoint (default `http://127.0.0.1:11434`).

## Quick Start

### Install

```bash
go install github.com/opus-domini/praetor/cmd/praetor@latest
```

### Or build locally

```bash
make build
./build/praetor --help
```

### Install praetor into your project

```bash
# Detects agents, registers MCP server, creates shared commands
praetor init
```

### Check agent availability

```bash
praetor doctor
```

### Create and run a plan

```bash
# Create a plan from a brief (agent-assisted by default)
praetor plan create "Implement JWT auth with tests and docs"

# Or render a reusable template from project/global/builtin registry
praetor plan create --from-template go-feature \
  --var Name=auth \
  --var Summary="Implement JWT auth"

# Run plan (default runner: tmux)
praetor plan run implement-jwt-auth-with-tests-and-docs
```

### Run with cost and parallel guard rails

```bash
praetor plan run my-plan \
  --runner direct \
  --executor codex \
  --reviewer claude \
  --plan-cost-budget-usd 5 \
  --task-cost-budget-usd 1 \
  --max-parallel-tasks 2 \
  --max-retries 3 \
  --max-transitions 200
```

### Run with fallback

```bash
praetor plan run my-plan \
  --fallback-on-transient ollama \
  --fallback-on-auth openrouter
```

### Check status and resume

```bash
praetor plan status my-plan --verbose
praetor plan status my-plan
praetor plan list
praetor plan resume my-plan
```

### Export a plan bundle

```bash
praetor plan export my-plan
praetor plan export my-plan --output ./.praetor/exports/my-plan --force
```

### Diagnose a run

```bash
praetor plan diagnose my-plan
praetor plan diagnose my-plan --query errors --format json
praetor plan diagnose my-plan --query summary
```

### Evaluate quality (local)

```bash
praetor plan eval my-plan
praetor plan eval my-plan --run-id <run-id> --format json
praetor eval
praetor eval --window 168h --format json
make eval-plan PLAN_SLUG=my-plan
make eval
```

### Single prompt mode

```bash
praetor exec "Reply with OK"
praetor exec --provider claude "Summarize this diff"
praetor exec --provider ollama --model llama3 "Explain this module"
praetor exec --provider openrouter --model anthropic/claude-sonnet-4 "Review this code"
```

## Command Overview

| Command | Description |
|---|---|
| `praetor plan run <slug>` | Execute orchestration pipeline |
| `praetor plan create [brief]` | Create a plan from text/markdown input |
| `praetor plan export <slug>` | Export plan, state, summary, and reusable template |
| `praetor plan status <slug>` | Inspect state and progress |
| `praetor plan list` | List tracked plans for current project |
| `praetor plan show <slug>` | Print plan JSON to stdout |
| `praetor plan path <slug>` | Print absolute plan file path |
| `praetor plan edit <slug>` | Open a plan in `$EDITOR` |
| `praetor plan reset <slug>` | Clear runtime state for one plan |
| `praetor plan resume <slug>` | Restore latest valid local snapshot |
| `praetor plan diagnose <slug>` | Inspect structured diagnostics |
| `praetor plan eval <slug>` | Evaluate one local plan run (acceptance, gates, parse errors, stalls, retries, cost) |
| `praetor eval` | Aggregate latest local plan evaluations at project level |
| `praetor exec [prompt]` | Run a single prompt against one provider |
| `praetor doctor` | Check agent availability and health |
| `praetor config show` | Show effective config with source annotations |
| `praetor config set <key> <value>` | Persist a configuration key |
| `praetor config path` | Print resolved config file path |
| `praetor config edit` | Open config in `$EDITOR` |
| `praetor config init` | Create a commented template config file |
| `praetor init` | Install praetor into the current project |
| `praetor mcp` | Start MCP server over stdio |

## Configuration and State

- Config file: `$PRAETOR_CONFIG` > `<praetor-home>/config.toml` (TOML format).
- Config cascade: built-in defaults < global config < project section < plan settings < CLI flags.
- Home directory: `$PRAETOR_HOME` > `$XDG_CONFIG_HOME/praetor` > `~/.config/praetor`.
- All state is isolated per git project under `<home>/projects/<project-key>/`.
- Plans are identified by slug and stored in `<project>/plans/<slug>.json`.
- Plan templates are resolved from `<project-root>/.praetor/templates/`, `<praetor-home>/templates/`, then builtin templates.
- Structured feedback and runtime artifacts live under `<project>/feedback/<slug>/` and `<project>/runtime/<run-id>/`.
- Manifest discovery order: `praetor.yaml` > `praetor.yml` > `praetor.md`.

## Documentation

- [Documentation Home](https://opus-domini.github.io/praetor/#/)
- [Architecture](https://opus-domini.github.io/praetor/#/architecture)
- [Pipeline Orchestration](https://opus-domini.github.io/praetor/#/orchestration)
- [Configuration](https://opus-domini.github.io/praetor/#/configuration)
- [MCP Server](https://opus-domini.github.io/praetor/#/mcp)
- [Shared Agent Commands](https://opus-domini.github.io/praetor/#/commands)
- [Operations Runbook](https://opus-domini.github.io/praetor/#/operations-runbook)
- [Providers Overview](https://opus-domini.github.io/praetor/#/providers/README)
- [Claude Provider](https://opus-domini.github.io/praetor/#/providers/claude)
- [Codex Provider](https://opus-domini.github.io/praetor/#/providers/codex)
- [Copilot Provider](https://opus-domini.github.io/praetor/#/providers/copilot)
- [Gemini Provider](https://opus-domini.github.io/praetor/#/providers/gemini)
- [Kimi Provider](https://opus-domini.github.io/praetor/#/providers/kimi)
- [LM Studio Provider](https://opus-domini.github.io/praetor/#/providers/lmstudio)
- [OpenCode Provider](https://opus-domini.github.io/praetor/#/providers/opencode)
- [OpenRouter Provider](https://opus-domini.github.io/praetor/#/providers/openrouter)
- [Ollama Provider](https://opus-domini.github.io/praetor/#/providers/ollama)

## Development

```bash
make fmt              # Format code
make lint             # Lint code
make test             # Run tests
make test-coverage    # Run tests with race detection + coverage
make benchmark        # Run benchmarks
make security         # Run govulncheck
make ci               # Full CI pipeline (fmt + lint + test + coverage + benchmark + security)
```

## Stargazers over time ⭐

[![Stargazers over time](https://starchart.cc/opus-domini/praetor.svg?variant=adaptive)](https://starchart.cc/opus-domini/praetor)
