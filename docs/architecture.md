# Architecture

## Context

`praetor` is a CLI-first orchestrator with two complementary execution modes:

- Single prompt dispatch (`praetor run`)
- Plan-driven loop orchestration (`praetor loop run`)

The current providers are:

- Claude
- Codex

## Package boundaries

- `cmd/praetor`: command-line parsing, runtime wiring, and process exit behavior.
- `internal/loop`: immutable plan model, mutable state store, dependency graph evaluation, and loop runner pipeline.
- `internal/orchestrator`: provider contract (`Provider`), request/response types, provider registry, and dispatch engine.
- `internal/providers/claude`: Claude SDK port and orchestrator adapter.
- `internal/providers/codex`: Codex SDK port and orchestrator adapter.
- `docs/plans`: plan files for loop execution.
- `docs/schemas/loop-plan.schema.json`: JSON schema reference for plan authoring.
- `docs/providers`: canonical provider documentation.

## Execution flow

Single prompt mode:

1. CLI parses provider and prompt.
2. CLI builds one provider implementation.
3. Provider is registered in the orchestration registry.
4. Orchestration engine validates input and dispatches request.
5. Provider adapter translates between orchestrator contract and provider-specific SDK surface.

Loop mode:

1. CLI loads and validates a plan JSON file.
2. State store initializes or merges mutable state for the plan.
3. Runner selects the next runnable task based on dependency completion.
4. Executor agent runs task prompt and emits `RESULT: PASS|FAIL|UNKNOWN`.
5. Reviewer agent decides `PASS|reason` or `FAIL|reason`.
6. Runner marks task as done on pass, or increments retry and stores feedback on fail.
7. Loop exits when all tasks are done, blocked by dependencies, or retry/iteration limits are reached.
8. Every agent invocation executes inside a tmux session window for live operational visibility.

## Why this layout

- Keeps command handling separate from orchestration logic.
- Prevents provider internals from leaking outside the module (`internal/`).
- Enables adding providers incrementally without changing CLI core behavior.
- Preserves a clear migration path toward multi-provider, parallel scheduling, and richer execution gates.
- Keeps implementation folders lightweight while centralizing narrative docs in one place.
