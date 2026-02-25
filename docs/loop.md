# Loop orchestration

`praetor loop` is the Go-native replacement path for the bash loop scripts kept in `refs/ai-loop`.

## Commands

Create a new plan skeleton:

```bash
praetor loop plan new my-feature
```

Run a plan:

```bash
praetor loop run --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

Use a custom tmux session name:

```bash
praetor loop run --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json --tmux-session praetor-my-team
```

Inspect plan progress:

```bash
praetor loop plan status --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

List all known plan states:

```bash
praetor loop plan list
```

Reset one plan state:

```bash
praetor loop plan reset --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

## Runtime model

- Plan file is immutable input.
- State file is mutable output under `~/.praetor/projects/<project-hash>/state/`.
- Project scope is derived from the current git repository root.
- Agent execution always runs inside tmux windows in a dedicated session.
- Tasks are selected by dependency readiness.
- Retry counters and reviewer feedback are persisted per task signature.
- Each execution attempt writes logs under `~/.praetor/projects/<project-hash>/logs/`.

## Output style

- Terminal output is colorized by default when stdout is a TTY.
- Use `--no-color` to disable ANSI colors.

## Plan shape

Minimal example:

```json
{
  "$schema": "../schemas/loop-plan.schema.json",
  "title": "my feature",
  "tasks": [
    {
      "id": "TASK-001",
      "title": "Implement feature",
      "executor": "codex",
      "reviewer": "claude",
      "description": "Implement the requested feature in scoped files."
    },
    {
      "id": "TASK-002",
      "title": "Add tests",
      "depends_on": ["TASK-001"],
      "executor": "codex",
      "reviewer": "claude",
      "description": "Add tests for the implemented feature."
    }
  ]
}
```
