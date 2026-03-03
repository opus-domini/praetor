# Shared Agent Commands

Praetor generates shared agent commands that work across multiple AI coding agents (Claude Code, Cursor, Codex, etc.) from a single source of truth.

## How it works

Commands are stored centrally in `.agents/commands/` with symlinks from each agent's config directory:

```text
.agents/
  commands/
    plan-create.md      <- single definition
    plan-run.md
    review-task.md
    doctor.md
    diagnose.md

.claude/commands/  ->  ../.agents/commands/   (symlink)
.cursor/commands/  ->  ../.agents/commands/   (symlink)
.codex/commands/   ->  ../.agents/commands/   (symlink)
```

Updating one file updates all agents simultaneously. No drift between agent behaviors.

## CLI workflows

### Generate commands

```bash
praetor commands sync
praetor commands sync --agents claude,cursor
```

This creates `.agents/commands/` with 5 built-in commands and creates relative symlinks from each agent's directory.

### List commands

```bash
praetor commands list
```

Shows all available shared commands found in `.agents/commands/`.

## Built-in commands

| Command | Description | Allowed Tools |
|---|---|---|
| `plan-create` | Create a structured execution plan | Read, Glob, Grep, git |
| `plan-run` | Execute an existing praetor plan | praetor CLI only |
| `review-task` | Review executor output against criteria | Read, Grep, make test/lint |
| `doctor` | Check agent provider health | praetor doctor |
| `diagnose` | Debug a plan run | praetor CLI, Read |

Each command declares which tools the agent is allowed to use, implementing a **least-privilege model** for AI agents.

## Customization

Override any command by editing its `.md` file in `.agents/commands/`. Running `praetor commands sync` again will overwrite the file, so maintain customizations manually.

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

By default, symlinks are created for: `claude`, `cursor`, `codex`. Use the `--agents` flag to customize.

## Implementation

The commands package is implemented in `internal/commands/`:

- `commands.go` — `Sync()`, `List()`, `DefaultCommands()`
- `content.go` — Markdown content constants for each command
