# OpenRouter

Adapter for the [OpenRouter API](https://openrouter.ai). Uses the OpenAI-compatible Chat Completions endpoint.

## Overview

| Property | Value |
|---|---|
| Agent ID | `openrouter` |
| Transport | REST |
| Default base URL | `https://openrouter.ai/api/v1` |
| Default model | `openai/gpt-4o-mini` |
| Requires TTY | no |
| Structured output | yes |
| Auth | API key via environment variable |

## API integration

### Endpoint

```
POST {baseURL}/chat/completions
```

### Request payload

OpenAI-compatible Chat Completions format:

```json
{
  "model": "openai/gpt-4o-mini",
  "messages": [
    {"role": "user", "content": "<prompt>"}
  ],
  "stream": false
}
```

### Headers

```
Authorization: Bearer <apiKey>
Content-Type: application/json
```

The API key is read from the configured environment variable at call time (not cached at construction). Default env var: `OPENROUTER_API_KEY`.

### Response

```json
{
  "model": "openai/gpt-4o-mini",
  "choices": [
    {
      "message": {
        "content": "..."
      }
    }
  ]
}
```

The adapter extracts `choices[0].message.content` as the output and `model` as the resolved model name. Returns an error if the `choices` array is empty.

### Timeout

HTTP timeout: 5 minutes per request.

## Pipeline behavior

| Phase | Method | System prompt |
|---|---|---|
| Plan | `Plan()` | Concatenated via `ComposePrompt()` |
| Execute | `Execute()` | Concatenated via `ComposePrompt()` |
| Review | `Review()` | Concatenated via `ComposePrompt()` |

All phases use the same `generate()` path. System and user prompts are concatenated with `\n\n` separator into a single user message.

For `Plan()`, the JSON manifest is extracted from the response via `ExtractJSONObject()`.

For `Review()`, the adapter calls `ParseReview()` on the output.

## Health check

```
GET {defaultBaseURL}/api/v1/models
```

Returns `ok` on HTTP 2xx/3xx.

## Cost tracking

No cost information in the API response. `CostUSD` is always `0`.

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `openrouter-url` | `--openrouter-url` | `https://openrouter.ai/api/v1` | Base URL |
| `openrouter-model` | `--openrouter-model` | `openai/gpt-4o-mini` | Default model |
| `openrouter-api-key-env` | `--openrouter-api-key-env` | `OPENROUTER_API_KEY` | Env var for API key |

OpenRouter supports 300+ models from various providers. Use the model slug from the [OpenRouter model list](https://openrouter.ai/models) (e.g. `anthropic/claude-sonnet-4`, `google/gemini-2.5-pro`, `openai/gpt-4o`).
