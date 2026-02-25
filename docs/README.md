<div align="center">
    <img src="docs/assets/images/logo.png" alt="Logo Praetor" width="500"/>
    <hr />
    <p>Lead. Delegate. Dominate.</p>
</div>

# Praetor Documentation

`praetor` is a Go CLI that orchestrates AI agent providers through a single execution surface. It drives Claude Code and Codex as subprocess agents, coordinated by an executor/reviewer pipeline with git safety, cost tracking, and crash recovery.

## Core documentation

- [Architecture](architecture.md) — package boundaries, execution flow, and design rationale
- [Loop orchestration](loop.md) — plan format, runtime model, safety mechanisms, and CLI reference
- [Providers overview](providers/README.md) — how providers are abstracted and integrated

## Provider documentation

- [Claude provider](providers/claude.md) — Claude Code SDK port, stream-json transport, session control
- [Codex provider](providers/codex.md) — Codex SDK port, JSONL transport, thread model

## Documentation standard

- All technical documentation is written in English.
- `docs/` is the canonical source for project documentation, served with docsify.
- Provider-specific `README.md` files inside `internal/` are minimal pointers to canonical docs here.
