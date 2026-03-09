# Shared Agent Commands

Praetor generates shared agent commands that work across multiple AI coding agents (Claude Code, Cursor, Codex, etc.) from a single source of truth.

## Quickstart

```bash
praetor init
```

In an interactive terminal, `praetor init` shows a checkbox menu to select which agents to install. In non-interactive mode (CI, pipes), it auto-detects agent directories and falls back to all supported agents.

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

| Command | Description | Key capabilities |
|---|---|---|
| `praetor-plan-create` | Create a structured execution plan | All input modes (brief, file, stdin, template, wizard), `--dry-run`, `--no-agent`, full schema reference |
| `praetor-plan-run` | Execute a plan through the pipeline | Agent selection, cost budgets, parallel tasks, isolation, `--no-review`, post-run diagnostics |
| `praetor-review-task` | Review executor output against acceptance criteria | Structured JSON verdict (`{decision, reason, hints}`), quality gate checklist, actionable hints |
| `praetor-doctor` | Check agent provider health | `--json` output, `--timeout`, install instructions per provider |
| `praetor-diagnose` | Debug and evaluate plan runs | All diagnostic queries (errors, stalls, fallbacks, costs, summary, regressions), `plan eval`, `eval` |

Each command includes:

- **Usage examples** with real CLI flags and options
- **Key flags table** with defaults and descriptions
- **Workflow steps** for the complete task lifecycle
- **Allowed tools** declaration implementing a least-privilege model

## Customization

Override any command by editing its `.md` file in `.agents/commands/`. Running `praetor init` again will regenerate the files, so maintain customizations manually.

## Tool whitelisting pattern

Each slash command specifies exactly which tools the agent is allowed to use:

```markdown
## Allowed tools

Read, Glob, Grep, Bash(praetor plan create *), Bash(praetor plan show *)
```

This ensures:
- A plan-run command can only call praetor CLI tools
- A review command can only read files and run tests
- A plan-create command can analyze but not modify code directly

## Supported agents

`praetor init` supports three agent directories: `.claude/`, `.cursor/`, `.codex/`. In interactive mode, users select which agents to install via a checkbox TUI. In non-interactive mode, it auto-detects existing directories and falls back to all three.

## Implementation

The commands package is implemented in `internal/commands/`:

- `commands.go` — `Sync()`, `List()`, `DefaultCommands()`
- `content.go` — Markdown content for each command (plan-create, plan-run, review-task, doctor, diagnose)
