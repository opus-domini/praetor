# Kimi

Adapter for [Kimi CLI](https://kimi.ai).

## Overview

| Property | Value |
|---|---|
| Agent ID | `kimi` |
| Transport | CLI |
| Binary | `kimi` (configurable via `--kimi-bin` or `kimi-bin` config) |
| Requires TTY | yes |
| Structured output | no |
| Install | See https://kimi.ai |

## Command construction

All operations use a single execution path:

```
kimi \
  [--model <model>]
```

- Prompt is delivered via **stdin**.
- `UsePTY = true` — Kimi is interactive-first.
- No subcommand or `-p` flag — the bare binary name is invoked directly.
- System prompt is concatenated into the user prompt via `ComposePrompt()`.

There is no one-shot mode. `Execute()` always uses the stdin/PTY path.

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
| `kimi-bin` | `--kimi-bin` | `kimi` | Binary path or name |
