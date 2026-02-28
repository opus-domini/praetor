# Copilot

Adapter for [GitHub Copilot CLI](https://githubnext.com/projects/copilot-cli/) (`@githubnext/github-copilot-cli`).

## Overview

| Property | Value |
|---|---|
| Agent ID | `copilot` |
| Transport | CLI |
| Binary | `copilot` (configurable via `--copilot-bin` or `copilot-bin` config) |
| Requires TTY | no |
| Structured output | no |
| Install | `npm install -g @githubnext/github-copilot-cli` |

## Command construction

All operations use a single execution path:

```
copilot -p \
  [--model <model>] \
  --allow-all-tools \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false`.
- `--allow-all-tools` is always set.
- System prompt is concatenated into the user prompt via `ComposePrompt()`.

There is no distinction between pipeline and one-shot modes — `Execute()` always uses the same path.

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
| `copilot-bin` | `--copilot-bin` | `copilot` | Binary path or name |
