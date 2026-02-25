# Architecture

## Context

`praetor` is a CLI-first orchestrator that routes a single prompt to one provider implementation.

The current providers are:

- Claude
- Codex

## Package boundaries

- `cmd/praetor`: command-line parsing, runtime wiring, and process exit behavior.
- `internal/orchestrator`: provider contract (`Provider`), request/response types, provider registry, and dispatch engine.
- `internal/providers/claude`: Claude SDK port and orchestrator adapter.
- `internal/providers/codex`: Codex SDK port and orchestrator adapter.
- `docs/providers`: canonical provider documentation.

## Execution flow

1. CLI parses provider and prompt.
2. CLI builds exactly one provider implementation.
3. Provider is registered in the orchestration registry.
4. Orchestration engine validates input and dispatches request.
5. Provider adapter translates between orchestrator contract and provider-specific SDK surface.

## Why this layout

- Keeps command handling separate from orchestration logic.
- Prevents provider internals from leaking outside the module (`internal/`).
- Enables adding providers incrementally without changing CLI core behavior.
- Preserves a clear migration path toward multi-provider and parallel orchestration.
- Keeps implementation folders lightweight while centralizing narrative docs in one place.
