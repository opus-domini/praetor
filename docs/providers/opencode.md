# OpenCode

Adapter for [OpenCode](https://github.com/opencode-ai/opencode).

## Overview

| Property | Value |
|---|---|
| Agent ID | `opencode` |
| Transport | CLI |
| Binary | `opencode` (configurable via `--opencode-bin` or `opencode-bin` config) |
| Requires TTY | no |
| Structured output | no |
| Install | `go install github.com/opencode-ai/opencode@latest` |

## Command construction

All operations use a single execution path:

```
opencode run --quiet \
  [--model <model>] \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false`.
- Always uses the `run --quiet` subcommand.
- System prompt is concatenated into the user prompt via `ComposePrompt()`.

There is no one-shot mode. `Execute()` always uses the same path.

## Output parsing

Raw stdout, trimmed. No JSON parsing or structured output extraction.

## Pipeline behavior

| Phase | Method | System prompt |
|---|---|---|
| Plan | `Plan()` | Concatenated via `ComposePrompt()` |
| Execute | `Execute()` | Concatenated via `ComposePrompt()` |
| Review | `Review()` | Concatenated via `ComposePrompt()` |

For `Plan()`, the JSON manifest is extracted from raw output via `ExtractJSONObject()`.

For `Review()`, the adapter calls `ParseReview()` on the output.

## Cost tracking

No cost information available. `CostUSD` is always `0`.

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `opencode-bin` | `--opencode-bin` | `opencode` | Binary path or name |
