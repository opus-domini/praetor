# Shared Agent Commands

Praetor generates shared agent commands that work across multiple AI coding agents (Claude Code, Cursor, Codex, etc.) from a single source of truth.

## Quickstart

```bash
praetor init
```

`praetor init` detects installed agents, generates shared commands, and registers the MCP server — all in one step.

## How it works

Commands are stored centrally in `.agents/commands/` with symlinks from each agent's config directory:

```text
.agents/
  commands/
    praetor-plan-create.md      <- single definition
    praetor-plan-run.md
    praetor-review-task.md
    praetor-doctor.md
    praetor-diagnose.md

.claude/commands/  ->  ../.agents/commands/   (symlink)
.cursor/commands/  ->  ../.agents/commands/   (symlink)
.codex/commands/   ->  ../.agents/commands/   (symlink)
```

Updating one file updates all agents simultaneously. No drift between agent behaviors.

## Built-in commands

| Command | Description | Allowed Tools |
|---|---|---|
| `praetor-plan-create` | Create a structured execution plan from a brief or reusable template | Read, Glob, Grep, git |
| `praetor-plan-run` | Execute an existing praetor plan with review, gates, and runtime policies | praetor CLI only |
| `praetor-review-task` | Review executor output against criteria | Read, Grep, make test/lint |
| `praetor-doctor` | Check agent provider health with structured environment diagnostics | praetor doctor |
| `praetor-diagnose` | Debug a plan run with summary, actor, and cost diagnostics | praetor CLI, Read |

Operational notes:

- `praetor-plan-create` can drive `praetor plan create --from-template <name>` when the request maps to a reusable plan scaffold.
- `praetor-diagnose` can query `summary` to inspect actor-level retries, stalls, and cost distribution.
- `praetor-doctor` surfaces binary paths, endpoint details, parsed versions, and hints from `checks[]`.

Each command declares which tools the agent is allowed to use, implementing a **least-privilege model** for AI agents.

## Customization

Override any command by editing its `.md` file in `.agents/commands/`. Running `praetor init` again will regenerate the files, so maintain customizations manually.

## Tool whitelisting pattern

Each slash command specifies exactly which tools the agent can use:

```markdown
## Allowed Tools

Read, Edit, Glob, Grep, Bash(make test), Bash(make lint)
```

This ensures:
- A task command can only call MCP APIs
- A review command can only read files and run tests
- A plan command can analyze but not modify code

## Supported agents

`praetor init` detects agent directories (`.claude/`, `.cursor/`, `.codex/`) present in the project. When none are found, it creates symlinks for all supported agents by default.

## Implementation

The commands package is implemented in `internal/commands/`:

- `commands.go` — `Sync()`, `List()`, `DefaultCommands()`
- `content.go` — Markdown content constants for each command
