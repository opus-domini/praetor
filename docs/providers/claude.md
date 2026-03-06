# Claude

Adapter for [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`@anthropic-ai/claude-code`). Uses provider-native structured output for planning and `stream-json` for pipeline execution/review with cost tracking.

## Overview

| Property | Value |
|---|---|
| Agent ID | `claude` |
| Transport | CLI |
| Binary | `claude` (configurable via `--claude-bin` or `claude-bin` config) |
| Requires TTY | yes |
| Structured output | yes |
| Install | `npm install -g @anthropic-ai/claude-code` |

## Command construction

### Planner path (`Plan()`)

```
claude -p \
  --no-session-persistence \
  --verbose \
  --output-format json \
  --tools "" \
  --disable-slash-commands \
  --json-schema '<plan-schema-json>' \
  [--model <model>] \
  [--append-system-prompt <systemPrompt>] \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false` to force a single validated result.
- `--json-schema` constrains the final response to the plan shape.
- `--tools ""` and `--disable-slash-commands` prevent planner-side tool execution and skill expansion.
- Praetor intentionally does **not** use Claude's interactive plan mode, because that returns planning-event envelopes instead of a deterministic planner payload.

### Streaming path (Review and Execute in pipeline)

```
claude -p \
  --dangerously-skip-permissions \
  --no-session-persistence \
  --verbose \
  --output-format stream-json \
  [--model <model>] \
  [--append-system-prompt <systemPrompt>]
```

- Prompt is delivered via **stdin** (not as a positional argument).
- `UsePTY = true` — requires a PTY for the streaming protocol.
- `--dangerously-skip-permissions` and `--no-session-persistence` are always set.
- System prompt uses the `--append-system-prompt` flag (not concatenated into the user prompt).

### One-shot path (Execute with `OneShot = true`)

```
claude -p \
  --output-format json \
  [--model <model>] \
  [--append-system-prompt <systemPrompt>] \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false` — single JSON response, no streaming.
- Used by `praetor exec` for quick single-dispatch invocations.

## Output parsing

### Streaming (`stream-json`)

The adapter reads a JSONL stream from stdout. Each line is a JSON event with a `type` field.

1. Scans for `type == "result"` events — extracts `.result` text and `.cost_usd`.
2. If no result text found, falls back to collecting all `type == "assistant"` content blocks where `block.type == "text"`.
3. Model name is extracted from result events when available.

### One-shot (`json`)

Claude returns a result envelope. When schema output is enabled, the final structured payload is carried in `structured_output`:

```json
{
  "type": "result",
  "result": "...",
  "structured_output": { "...": "..." },
  "model": "...",
  "cost_usd": 0.042
}
```

Falls back to raw stdout if JSON parsing fails.

## Pipeline behavior

| Phase | Method | Path | System prompt |
|---|---|---|---|
| Plan | `Plan()` | One-shot JSON schema | `adapter.plan.claude.tmpl` via `--append-system-prompt` |
| Execute | `Execute()` | Streaming | Via `--append-system-prompt` if provided |
| Execute (one-shot) | `Execute(OneShot=true)` | One-shot JSON | Via `--append-system-prompt` if provided |
| Review | `Review()` | Streaming | Via `--append-system-prompt` if provided |

For `Plan()`, the prompt is rendered using the `adapter.plan.claude.tmpl` template (or hardcoded fallback if no prompt engine is available). Praetor does not rely only on prompt wording: it also enforces Claude's native `--json-schema` contract and disables planner tools.

For `Review()`, the adapter calls `ParseReview()` on the output to extract `DECISION: PASS/FAIL` and `REASON:` lines.

## Cost tracking

Claude's `json` and `stream-json` formats include `cost_usd`. This is propagated through `PlanResponse.CostUSD`, `ExecuteResponse.CostUSD`, and `ReviewResponse.CostUSD` for the metrics middleware and cost ledger.

## tmux integration

When running under the tmux runner, the wrapper script includes `unset CLAUDECODE` to allow nested Claude Code sessions (otherwise the inner `claude` process detects the parent and refuses to start).

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `claude-bin` | `--claude-bin` | `claude` | Binary path or name |

Model override is per-invocation via `--model` flag on the adapter command (driven by `--executor-model`, `--reviewer-model`, or `--planner-model` on the CLI).
