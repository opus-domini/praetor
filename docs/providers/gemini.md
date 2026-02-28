# Gemini

Adapter for [Gemini CLI](https://github.com/google-gemini/gemini-cli) (`@google/gemini-cli`).

## Overview

| Property | Value |
|---|---|
| Agent ID | `gemini` |
| Transport | CLI |
| Binary | `gemini` (configurable via `--gemini-bin` or `gemini-bin` config) |
| Requires TTY | yes |
| Structured output | no |
| Install | `npm install -g @google/gemini-cli` |

## Command construction

### Streaming path (Plan, Review, Execute in pipeline)

```
gemini -p \
  [--model <model>]
```

- Prompt is delivered via **stdin**.
- `UsePTY = true` — Gemini CLI requires a TTY for interactive mode.
- System prompt is concatenated into the user prompt via `ComposePrompt()`.

### One-shot path (Execute with `OneShot = true`)

```
gemini -p \
  [--model <model>] \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false`.

## Output parsing

Raw stdout, trimmed. No JSON parsing or structured output extraction.

## Pipeline behavior

| Phase | Method | Path | System prompt |
|---|---|---|---|
| Plan | `Plan()` | Streaming (stdin) | Concatenated via `ComposePrompt()` |
| Execute | `Execute()` | Streaming (stdin) | Concatenated via `ComposePrompt()` |
| Execute (one-shot) | `Execute(OneShot=true)` | One-shot (arg) | Concatenated via `ComposePrompt()` |
| Review | `Review()` | Streaming (stdin) | Concatenated via `ComposePrompt()` |

For `Plan()`, the JSON manifest is extracted from raw output via `ExtractJSONObject()`.

For `Review()`, the adapter calls `ParseReview()` on the output.

## Cost tracking

No cost information available. `CostUSD` is always `0`.

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `gemini-bin` | `--gemini-bin` | `gemini` | Binary path or name |
