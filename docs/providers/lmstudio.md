# LM Studio

Adapter for [LM Studio](https://lmstudio.ai). Uses the OpenAI-compatible REST API for running local models.

## Overview

| Property | Value |
|---|---|
| Agent ID | `lmstudio` |
| Transport | REST |
| Default base URL | `http://localhost:1234` |
| Default model | _(none — uses whatever model is loaded)_ |
| Requires TTY | no |
| Structured output | yes |
| Auth | Optional (`LMSTUDIO_API_KEY` env var) |

## API integration

### Endpoint

```
POST {baseURL}/v1/chat/completions
```

### Request payload

```json
{
  "model": "<model>",
  "messages": [{"role": "user", "content": "<prompt>"}],
  "stream": false
}
```

`stream: false` forces a single complete response (no token-by-token streaming).

### Response

```json
{
  "model": "...",
  "choices": [{"message": {"content": "..."}}]
}
```

The adapter extracts the first choice's `message.content` as the output.

### Authentication

Authentication is **optional**. When the `LMSTUDIO_API_KEY` environment variable (or the configured env var) is set and non-empty, the adapter sends an `Authorization: Bearer <key>` header. When the env var is empty or unset, no auth header is sent — this is the normal case for local LM Studio usage.

### Timeout

HTTP timeout: 5 minutes per request.

## Pipeline behavior

| Phase | Method | System prompt |
|---|---|---|
| Plan | `Plan()` | Concatenated via `ComposePrompt()` |
| Execute | `Execute()` | Concatenated via `ComposePrompt()` |
| Review | `Review()` | Concatenated via `ComposePrompt()` |

All phases use the same `generate()` path with the OpenAI chat completions format. System and user prompts are concatenated with `\n\n` separator into a single user message.

For `Plan()`, the JSON manifest is extracted from the response via `ExtractJSONObject()`.

For `Review()`, the adapter calls `ParseReview()` on the output.

## Health check

```
GET {baseURL}/v1/models
```

Returns `ok` on HTTP 2xx/3xx.

## Cost tracking

No cost information. `CostUSD` is always `0`. LM Studio runs locally — costs are infrastructure-only.

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `lmstudio-url` | `--lmstudio-url` | `http://localhost:1234` | Base URL |
| `lmstudio-model` | `--lmstudio-model` | _(empty)_ | Default model |
| `lmstudio-api-key-env` | `--lmstudio-api-key-env` | `LMSTUDIO_API_KEY` | Env var for API key (optional) |

Load a model in LM Studio before using it with Praetor. The `--lmstudio-model` flag can be left empty if only one model is loaded — LM Studio will use it by default.
