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
It executes dependency-aware plans with isolated worktrees, independent review gates, snapshot-based recovery, and explicit runtime strategy tracking.

<p align="center">
  <a href="https://opus-domini.github.io/praetor/">Documentation</a> •
  <a href="https://github.com/opus-domini/praetor/releases">Releases</a> •
  <a href="#quick-start">Quick Start</a>
</p>

## Why Praetor

- One CLI surface for planning, execution, review, and recovery.
- Unified provider abstraction across CLI and REST agents.
- Explicit finite-state orchestration with transition guard rails.
- Worktree-first isolation to protect the main branch during task execution.
- Local transactional snapshots with checksum validation and explicit resume.
- Observable execution with checkpoints, metrics, and runtime strategy logging.

## Core Capabilities

- **Plan execution** — run JSON plans with dependencies via `praetor plan run`.
- **Agents** — built-in `codex`, `claude`, `gemini`, and `ollama` backends.
- **Plan-and-Execute** — optional planner phase (`--objective`) followed by execute/review gates.
- **FSM runtime** — loop modeled as explicit states with `max-iterations` and `max-transitions` guard rails.
- **Runner modes** — `tmux`, `direct`, and `pty` under a unified runtime contract.
- **PTY fallback** — execution strategy is recorded as `structured`, `process`, or `pty`.
- **Workspace context** — automatic manifest discovery from `praetor.yaml` / `praetor.md`.
- **Recovery** — automatic snapshot inspection plus manual `praetor plan resume`.
- **Retention** — local runtime pruning with `--keep-last-runs`.
- **Auditability** — checkpoint history, cost ledger, and per-task logs.

## Requirements

- Linux or macOS.
- Go 1.26+.
- `git` available in `PATH`.
- For `--runner tmux`: `tmux` installed.
- Agent binaries as needed: `codex`, `claude`, `gemini`.
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

### Create and run a plan

```bash
# Create a plan from a brief (agent-assisted by default)
praetor plan create "Implement JWT auth with tests and docs"

# Run plan (default runner: tmux)
praetor plan run implement-jwt-auth-with-tests-and-docs
```

### Run with direct mode (no tmux)

```bash
praetor plan run my-plan \
  --runner direct \
  --executor codex \
  --reviewer claude \
  --max-retries 3 \
  --max-transitions 200
```

### Check status and resume

```bash
praetor plan status my-plan
praetor plan list
praetor plan resume my-plan
```

### Single prompt mode

```bash
praetor exec "Reply with OK"
praetor exec --provider claude "Summarize this diff"
praetor exec --provider ollama --model llama3.1 "Explain this module"
```

## Command Overview

- `praetor plan run <slug>` — execute orchestration pipeline.
- `praetor plan status <slug>` — inspect state/progress.
- `praetor plan list` — list tracked plans for current project.
- `praetor plan create [brief]` — create a plan from text/markdown input.
- `praetor plan edit <slug>` — open a plan in `$EDITOR`.
- `praetor plan show <slug>` — print plan JSON to stdout.
- `praetor plan path <slug>` — print the absolute plan file path.
- `praetor plan reset <slug>` — clear runtime state for one plan.
- `praetor plan resume <slug>` — restore latest valid local snapshot.
- `praetor plan diagnose <slug>` — inspect structured diagnostics (`events.jsonl`, `performance.jsonl`).
- `praetor exec [prompt]` — run a single prompt against one provider.

## Configuration and State

- Home directory: `$PRAETOR_HOME` > `$XDG_CONFIG_HOME/praetor` > `~/.config/praetor`.
- All state is isolated per git project under `<home>/projects/<project-key>/`.
- Plans are identified by slug and stored in `<project>/plans/<slug>.json`.
- Manifest discovery order: `praetor.yaml` > `praetor.yml` > `praetor.md`.

## Documentation

- [Documentation Home](https://opus-domini.github.io/praetor/#/)
- [Architecture](https://opus-domini.github.io/praetor/#/architecture)
- [Pipeline Orchestration](https://opus-domini.github.io/praetor/#/orchestration)
- [Providers Overview](https://opus-domini.github.io/praetor/#/providers/README)
- [Claude Provider](https://opus-domini.github.io/praetor/#/providers/claude)
- [Codex Provider](https://opus-domini.github.io/praetor/#/providers/codex)

## Development

```bash
make fmt
make lint
make test
make ci
```

## Stargazers over time ⭐

[![Stargazers over time](https://starchart.cc/opus-domini/praetor.svg?variant=adaptive)](https://starchart.cc/opus-domini/praetor)
