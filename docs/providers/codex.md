# Codex Provider

Go port of `@openai/codex-sdk` integrated into the `praetor` orchestration model.

## Current coverage

- `Codex` client with `startThread`/`resumeThread` equivalents:
  - `New(...)`
  - `StartThread(...)`
  - `ResumeThread(...)`
- turn execution:
  - `Thread.Run(...)`
  - `Thread.RunStreamed(...)`
- JSONL event parsing (`codex exec --experimental-json`)
- support for canonical stream items:
  - `agent_message`
  - `reasoning`
  - `command_execution`
  - `file_change`
  - `mcp_tool_call`
  - `web_search`
  - `todo_list`
  - `error`
- support for:
  - text input
  - structured input with local images (`[]UserInput`)
  - output schema via `--output-schema`
  - thread options (`model`, `sandbox`, `cd`, `web-search`, `approval`, `add-dir`)
  - repeated `--config key=value` with nested object flattening

## Adapter integration

This provider is adapted to the orchestrator contract through:

- `NewProvider(CodexOptions)`
- `Provider.Run(ctx, orchestrator.Request)`
