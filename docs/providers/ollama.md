# Ollama

Adapter for [Ollama](https://ollama.com). Uses the local REST API for running open-source models.

## Overview

| Property | Value |
|---|---|
| Agent ID | `ollama` |
| Transport | REST |
| Default base URL | `http://127.0.0.1:11434` |
| Default model | `llama3` |
| Requires TTY | no |
| Structured output | no |
| Auth | None (local service) |

## API integration

### Endpoint

```
POST {baseURL}/api/generate
```

### Request payload

```json
{
  "model": "llama3",
  "prompt": "<prompt>",
  "stream": false
}
```

`stream: false` forces a single complete response (no token-by-token streaming).

### Response

```json
{
  "response": "..."
}
```

The adapter extracts the `.response` field as the output.

### Timeout

HTTP timeout: 5 minutes per request.

## Pipeline behavior

| Phase | Method | System prompt |
|---|---|---|
| Plan | `Plan()` | Concatenated via `ComposePrompt()` |
| Execute | `Execute()` | Concatenated via `ComposePrompt()` |
| Review | `Review()` | Concatenated via `ComposePrompt()` |

All phases use the same `generate()` path. System and user prompts are concatenated with `\n\n` separator into the `prompt` field.

For `Plan()`, the JSON manifest is extracted from the response via `ExtractJSONObject()`.

For `Review()`, the adapter calls `ParseReview()` on the output.

## Health check

```
GET {baseURL}/api/tags
```

Returns `ok` on HTTP 2xx/3xx.

## Cost tracking

No cost information. `CostUSD` is always `0`. Ollama runs locally — costs are infrastructure-only.

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `ollama-url` | `--ollama-url` | `http://127.0.0.1:11434` | Base URL |
| `ollama-model` | `--ollama-model` | `llama3` | Default model |

Any model available in your local Ollama installation can be used. Pull models with `ollama pull <model>` before using them with Praetor.
