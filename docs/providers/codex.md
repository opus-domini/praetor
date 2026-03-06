# Codex

Adapter for [OpenAI Codex CLI](https://github.com/openai/codex) (`@openai/codex`). Uses the `exec --json` subcommand for structured JSONL output and `--output-schema` for planner enforcement.

## Overview

| Property | Value |
|---|---|
| Agent ID | `codex` |
| Transport | CLI |
| Binary | `codex` (configurable via `--codex-bin` or `codex-bin` config) |
| Requires TTY | no |
| Structured output | yes |
| Install | `npm install -g @openai/codex` |

## Command construction

### Planner path (`Plan()`)

```
codex exec --json \
  --sandbox read-only \
  --skip-git-repo-check \
  --config approval_policy="never" \
  --ephemeral \
  --output-schema <schema-file> \
  --output-last-message <result-file> \
  [--cd <workdir>] \
  [--model <model>] \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false` — Codex doesn't require a TTY.
- `--sandbox read-only` prevents the planner from mutating the workspace.
- `--output-schema` constrains the final response to the plan shape.
- `--output-last-message` gives Praetor a deterministic final payload even while stdout remains JSONL.
- `--ephemeral` avoids persisting planner sessions to disk.

### Pipeline path (Review and Execute in pipeline)

```
codex exec --json \
  --sandbox workspace-write \
  --skip-git-repo-check \
  --config approval_policy="never" \
  [--cd <workdir>] \
  [--model <model>] \
  <prompt>
```

- Prompt is a positional argument.
- `UsePTY = false` — Codex doesn't require a TTY.
- `--sandbox workspace-write` restricts file system access.
- `--skip-git-repo-check` prevents git repo validation.
- `--config approval_policy="never"` disables interactive tool approval.
- Working directory is passed via `--cd` (not the process CWD).
- System prompt is concatenated into the user prompt via `ComposePrompt()`.

### One-shot path (Execute with `OneShot = true`)

```
codex exec --json \
  [--cd <workdir>] \
  [--model <model>] \
  <prompt>
```

- Same structure but without sandbox/approval flags.
- Used by `praetor exec` for quick single-dispatch invocations.

## Output parsing

Codex produces a JSONL event stream on stdout. The adapter parses it as follows:

1. Scans for events with `type == "item.completed"` where `item.type == "agent_message"`.
2. Extracts `item.text` from each matching event.
3. Tracks the `model` field from any event in the stream.
4. If the output isn't valid JSONL or no matching events are found, falls back to raw stdout.

Multiple `agent_message` items are concatenated with newlines.

## Pipeline behavior

| Phase | Method | Path | System prompt |
|---|---|---|---|
| Plan | `Plan()` | JSONL + output schema | Concatenated into prompt via `ComposePrompt()` |
| Execute | `Execute()` | Pipeline | Concatenated into prompt via `ComposePrompt()` |
| Execute (one-shot) | `Execute(OneShot=true)` | One-shot | Concatenated into prompt via `ComposePrompt()` |
| Review | `Review()` | Pipeline | Concatenated into prompt via `ComposePrompt()` |

For `Plan()`, the prompt is rendered using the `adapter.plan.tmpl` template (or hardcoded fallback). Praetor uses Codex's native `--output-schema` contract plus `--output-last-message` to capture the final plan payload deterministically.

For `Review()`, the adapter calls `ParseReview()` on the output to extract `DECISION: PASS/FAIL` and `REASON:` lines.

## Cost tracking

Codex's JSONL stream does not include cost information. `CostUSD` is always `0` in responses.

## Configuration

| Config key | CLI flag | Default | Description |
|---|---|---|---|
| `codex-bin` | `--codex-bin` | `codex` | Binary path or name |

Model override is per-invocation via `--model` flag on the adapter command.
