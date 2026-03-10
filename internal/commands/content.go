package commands

const planCreateContent = `# Plan Create

Create a structured, dependency-aware execution plan using praetor.

## Workflow

1. Understand the objective — read relevant code, docs, and project conventions
2. Run ` + "`praetor plan create`" + ` with the brief describing what needs to be done
3. Review the generated plan and iterate if needed

## Usage

` + "```bash" + `
# Interactive wizard (when no arguments given)
praetor plan create

# From a text brief — planner agent generates the full plan
praetor plan create "Refactor auth middleware to use JWT tokens"

# From a file
praetor plan create --from-file brief.md

# From stdin
echo "Add caching layer" | praetor plan create --stdin

# From a built-in template (bug-fix, feature, refactor, discovery, etc.)
praetor plan create --from-template feature --var name="user-profiles"

# Skip the planner agent — generate a minimal skeleton to edit manually
praetor plan create --no-agent "Add pagination to list endpoints"

# Preview without saving
praetor plan create --dry-run "Migrate database schema"
` + "```" + `

## Key flags

| Flag | Description |
|---|---|
| ` + "`--planner`" + ` | Planner agent (default: claude) |
| ` + "`--executor`" + ` | Default executor written into the plan |
| ` + "`--reviewer`" + ` | Default reviewer written into the plan |
| ` + "`--from-template`" + ` | Use a built-in template: bug-fix, discovery, feature, implementation, refactor, release, validation |
| ` + "`--var key=value`" + ` | Template variables (repeatable) |
| ` + "`--no-agent`" + ` | Generate skeleton without calling planner |
| ` + "`--dry-run`" + ` | Print plan JSON without saving |
| ` + "`--slug`" + ` | Explicit slug override |
| ` + "`--force`" + ` | Overwrite existing plan file |
| ` + "`--planner-model`" + ` | Model override for the planner agent |
| ` + "`--planner-timeout`" + ` | Timeout for plan generation (e.g. 5m) |

## Plan structure

The generated plan follows this schema:

- **name** — short kebab-case identifier
- **summary** — one-line description of the plan
- **cognitive** — assumptions, open_questions, failure_modes, decisions
- **settings.agents** — executor and reviewer (required), planner (optional)
- **settings.execution_policy** — max_retries_per_task, max_parallel_tasks, timeout, cost budgets, stall detection
- **quality.commands** — tests, lint, standards commands for quality gates
- **tasks[]** — ordered list of tasks, each with:
  - **id** — unique identifier (e.g. TASK-001)
  - **title** — short description
  - **description** — detailed instructions for the executor
  - **depends_on** — list of task IDs that must complete first
  - **acceptance** — list of verifiable acceptance criteria (at least one)
  - **agents** — optional per-task executor/reviewer overrides

## Tips

- Write clear, verifiable acceptance criteria — the reviewer uses them to decide PASS/FAIL
- Use depends_on to express ordering; independent tasks run in parallel automatically
- Keep tasks atomic — one concern per task makes review and retry more effective
- After creation, review with ` + "`praetor plan show <slug>`" + ` and edit with ` + "`praetor plan edit <slug>`" + `

## Allowed tools

Read, Glob, Grep, Bash(praetor plan create *), Bash(praetor plan show *), Bash(praetor plan edit *), Bash(praetor plan list)
`

const planRunContent = `# Plan Run

Execute an existing praetor plan through the executor/reviewer pipeline.

## Workflow

1. List available plans and pick one to run
2. Verify its current status
3. Execute the plan with appropriate flags
4. Monitor progress and report the outcome

## Usage

` + "```bash" + `
# List plans for the current project
praetor plan list

# Check plan status before running
praetor plan status <slug>

# Run a plan (default: parallel execution with worktree isolation)
praetor plan run <slug>

# Run without code review gate
praetor plan run <slug> --no-review

# Run with specific agents
praetor plan run <slug> --executor claude --reviewer codex

# Run with cost budget limits
praetor plan run <slug> --plan-cost-budget-usd 5.00 --task-cost-budget-usd 1.00

# Run sequentially (one task at a time)
praetor plan run <slug> --max-parallel-tasks 1

# Resume a previously interrupted run
praetor plan resume <slug>

# Run with an overall timeout
praetor plan run <slug> --timeout 30m
` + "```" + `

## Key flags

| Flag | Default | Description |
|---|---|---|
| ` + "`--executor`" + ` | codex | Executor agent |
| ` + "`--reviewer`" + ` | claude | Reviewer agent (use "none" to skip) |
| ` + "`--no-review`" + ` | false | Skip reviewer gate entirely |
| ` + "`--max-parallel-tasks`" + ` | 5 | Concurrent independent tasks per wave |
| ` + "`--max-retries`" + ` | 3 | Max retries per failed task |
| ` + "`--isolation`" + ` | worktree | Isolation mode: worktree or off |
| ` + "`--runner`" + ` | tmux | Runner mode: tmux, pty, or direct |
| ` + "`--timeout`" + ` | 0 (none) | Overall run timeout (e.g. 30m, 2h) |
| ` + "`--force`" + ` | false | Override an existing plan lock |
| ` + "`--plan-cost-budget-usd`" + ` | 0 | Plan-level cost budget in USD |
| ` + "`--task-cost-budget-usd`" + ` | 0 | Per-task cost budget in USD |
| ` + "`--objective`" + ` | — | Trigger macro-planning before execution |
| ` + "`--hook`" + ` | — | Script to run after executor, before reviewer |

## After the run

` + "```bash" + `
# Check final status
praetor plan status <slug>

# Diagnose failures
praetor plan diagnose <slug>

# Evaluate execution quality
praetor plan eval <slug>

# Reset state to re-run from scratch
praetor plan reset <slug>
` + "```" + `

## Allowed tools

Bash(praetor plan list), Bash(praetor plan status *), Bash(praetor plan run *), Bash(praetor plan resume *), Bash(praetor plan diagnose *), Bash(praetor plan eval *), Bash(praetor plan reset *)
`

const reviewTaskContent = `# Review Task

Review the output of a praetor task execution against its acceptance criteria.

## Workflow

1. Read the task's acceptance criteria from the plan
2. Examine the executor's changes (git diff, new/modified files)
3. Run quality gate commands (tests, lint) if defined
4. Verify each acceptance criterion is satisfied
5. Return a structured verdict

## Review checklist

- [ ] Every acceptance criterion is verifiably met
- [ ] Tests pass (run the project's test command)
- [ ] Linter passes (run the project's lint command)
- [ ] Changes follow project conventions (check AGENTS.md or CLAUDE.md)
- [ ] No unintended side effects or regressions
- [ ] No security vulnerabilities introduced (OWASP top 10)

## Verdict format

Return your verdict as a JSON object on the first line of your response:

` + "```json" + `
{"decision": "PASS", "reason": "All acceptance criteria met, tests pass", "hints": []}
` + "```" + `

Or for failures:

` + "```json" + `
{"decision": "FAIL", "reason": "Tests broken", "hints": ["Fix TestUserCreate assertion", "Add missing error handling in handler.go"]}
` + "```" + `

**Fields:**
- **decision** — ` + "`PASS`" + ` or ` + "`FAIL`" + ` (required)
- **reason** — concise explanation of the verdict (required)
- **hints** — actionable fix suggestions for the executor on retry (required for FAIL; include specific file names, function names, and what to fix)

## Important

- Be strict but fair — reject only when criteria are genuinely unmet
- Provide actionable hints on FAIL so the executor can fix issues on retry
- Do not approve code that introduces test failures or lint errors
- When unsure, verify by running the actual commands rather than guessing

## Allowed tools

Read, Glob, Grep, Bash(make test), Bash(make lint), Bash(git diff), Bash(git status), Bash(go test ./...), Bash(golangci-lint run)
`

const doctorContent = `# Doctor

Check the health and availability of all AI agent providers configured for praetor.

## Workflow

1. Run the doctor command
2. Review which agents are available and which are missing
3. Suggest fixes for any unavailable agents

## Usage

` + "```bash" + `
# Standard check with formatted output
praetor doctor

# Machine-readable output
praetor doctor --json

# Custom timeout for slow connections
praetor doctor --timeout 30s
` + "```" + `

## Reading the output

Each agent shows:
- **Transport** — [CLI] for local binaries, [REST] for API endpoints
- **Status** — pass (available), warn (issues), fail (unavailable)
- **Details** — version, binary path, endpoint URL, or install hint

## Common fixes

| Agent | Fix |
|---|---|
| Claude Code | ` + "`npm install -g @anthropic-ai/claude-code`" + ` |
| Codex CLI | ` + "`npm install -g @openai/codex`" + ` |
| Gemini CLI | ` + "`npm install -g @google/gemini-cli`" + ` |
| OpenCode | ` + "`go install github.com/opencode-ai/opencode@latest`" + ` |
| OpenRouter | Set ` + "`OPENROUTER_API_KEY`" + ` environment variable |
| Ollama | Start the Ollama server: ` + "`ollama serve`" + ` |
| LM Studio | Start LM Studio local server |

## Allowed tools

Bash(praetor doctor), Bash(praetor doctor --json)
`

const workflowContent = `# Workflow

End-to-end guide for orchestrating work with praetor — from plan creation through execution, monitoring, and diagnostics.

## Phase 1 — Preparation

Before creating a plan, check which agents are available:

` + "```bash" + `
praetor doctor
` + "```" + `

## Phase 2 — Plan creation

Create a plan from a brief describing the objective. The planner agent generates a structured plan with tasks, dependencies, and acceptance criteria.

` + "```bash" + `
# From a text brief (planner agent generates the plan)
praetor plan create "Implement JWT auth with refresh tokens"

# From a file with detailed requirements
praetor plan create --from-file brief.md

# Interactive wizard — choose planner, executor, reviewer interactively
praetor plan create

# From a reusable template
praetor plan create --from-template feature --var name="user-profiles"

# Quick skeleton without calling the planner agent
praetor plan create --no-agent "Add pagination to list endpoints"
` + "```" + `

After creation, review and optionally edit:

` + "```bash" + `
praetor plan show <slug>       # inspect the JSON
praetor plan edit <slug>       # open in $EDITOR
` + "```" + `

## Phase 3 — Execution

Run the plan. Each task goes through an executor agent, then an independent reviewer agent that gates promotion. Failed tasks retry with structured feedback.

` + "```bash" + `
# Run with defaults (tmux runner, worktree isolation, 5 parallel tasks)
praetor plan run <slug>

# Run with specific agents
praetor plan run <slug> --executor claude --reviewer codex

# Run without review gate (faster, less safe)
praetor plan run <slug> --no-review

# Run with cost budgets
praetor plan run <slug> --plan-cost-budget-usd 5.00 --task-cost-budget-usd 1.00

# Run with fallback agents for resilience
praetor plan run <slug> --fallback-on-transient ollama --fallback-on-auth openrouter

# Run with direct runner (no tmux, good for CI/scripts)
praetor plan run <slug> --runner direct
` + "```" + `

## Phase 4 — Monitoring

While a plan runs, check progress with status and events:

` + "```bash" + `
# High-level progress (done/failed/active/total)
praetor plan status <slug>

# Verbose status with per-task detail
praetor plan status <slug> --verbose

# List all plans and their current state
praetor plan list
` + "```" + `

## Phase 5 — Diagnostics (if needed)

When tasks fail or costs seem high, diagnose the run:

` + "```bash" + `
# Full diagnostics
praetor plan diagnose <slug>

# Targeted queries
praetor plan diagnose <slug> --query errors      # what failed and why
praetor plan diagnose <slug> --query stalls       # stuck retry loops
praetor plan diagnose <slug> --query fallbacks    # agent fallback events
praetor plan diagnose <slug> --query costs        # cost breakdown per actor
praetor plan diagnose <slug> --query summary      # high-level run summary

# Evaluate execution quality
praetor plan eval <slug>
praetor eval                                       # project-level aggregate
` + "```" + `

## Phase 6 — Recovery

If a run was interrupted or needs a fresh start:

` + "```bash" + `
# Resume from the latest valid snapshot
praetor plan resume <slug>

# Reset all state and re-run from scratch
praetor plan reset <slug>
praetor plan run <slug>
` + "```" + `

## MCP integration

When praetor is configured as an MCP server (via ` + "`praetor init`" + ` or ` + "`.mcp.json`" + `), all phases above are available as MCP tools. This enables any MCP-aware agent to orchestrate plans programmatically.

### MCP tool mapping

| Phase | CLI command | MCP tool |
|---|---|---|
| Preparation | ` + "`praetor doctor`" + ` | ` + "`doctor`" + ` |
| Plan creation | ` + "`praetor plan create`" + ` | ` + "`plan_create`" + ` (skeleton only) |
| Inspect plan | ` + "`praetor plan show <slug>`" + ` | ` + "`plan_show`" + ` |
| Execution | ` + "`praetor plan run <slug>`" + ` | ` + "`plan_run`" + ` (background) |
| Monitoring | ` + "`praetor plan status <slug>`" + ` | ` + "`plan_status`" + ` |
| Event stream | — | ` + "`plan_events`" + ` |
| Diagnostics | ` + "`praetor plan diagnose <slug>`" + ` | ` + "`plan_diagnose`" + ` |
| Config | ` + "`praetor config show`" + ` | ` + "`config_show`" + ` |
| Single prompt | ` + "`praetor exec`" + ` | ` + "`exec`" + ` |

### MCP workflow example

` + "```" + `
1. Call doctor to verify agent availability
2. Call plan_create with a name → get slug and path
3. Edit the plan file at the returned path (add tasks, acceptance criteria)
4. Call plan_show to verify the plan looks correct
5. Call plan_run with the slug → execution starts in background
6. Poll plan_status periodically to monitor progress
7. Call plan_events to stream execution events in real time
8. On completion, call plan_diagnose for a summary
` + "```" + `

### MCP resources

These resources provide passive data access without tool calls:

| Resource URI | Description |
|---|---|
| ` + "`praetor://plans`" + ` | List of all plans |
| ` + "`praetor://plans/{slug}`" + ` | Full plan JSON |
| ` + "`praetor://plans/{slug}/state`" + ` | Current execution state |
| ` + "`praetor://config`" + ` | Resolved configuration |
| ` + "`praetor://agents`" + ` | Agent health status |

## Allowed tools

Read, Glob, Grep, Bash(praetor *)
`

const diagnoseContent = `# Diagnose

Inspect and debug a praetor plan run to understand failures, costs, and performance.

## Workflow

1. Identify the plan to diagnose
2. Check current execution status
3. Run targeted diagnostic queries
4. Analyze results and suggest fixes

## Usage

` + "```bash" + `
# List plans for the current project
praetor plan list

# Check plan status
praetor plan status <slug>

# Full diagnostics (all queries)
praetor plan diagnose <slug>

# Targeted queries
praetor plan diagnose <slug> --query errors     # Task errors and failure reasons
praetor plan diagnose <slug> --query stalls      # Stall detection events
praetor plan diagnose <slug> --query fallbacks   # Agent fallback events
praetor plan diagnose <slug> --query costs       # Per-task and total cost breakdown
praetor plan diagnose <slug> --query summary     # High-level run summary

# Machine-readable output
praetor plan diagnose <slug> --format json

# Diagnose a specific run (not the latest)
praetor plan diagnose <slug> --run-id <run-id>

# Compare against a baseline for regressions
praetor plan diagnose <slug> --query regressions --baseline baseline.json
` + "```" + `

## Evaluate execution quality

` + "```bash" + `
# Evaluate a single plan run
praetor plan eval <slug>
praetor plan eval <slug> --format json
praetor plan eval <slug> --fail-on-fail  # non-zero exit on failure

# Evaluate all plans in the project
praetor eval
praetor eval --window 72h  # only plans run in last 72 hours
` + "```" + `

## Diagnosis checklist

When investigating failures:

1. **Errors** — which tasks failed and why? Check executor output and reviewer feedback
2. **Stalls** — did any task get stuck in a retry loop producing similar output?
3. **Costs** — is spending within budget? Any single task consuming disproportionate cost?
4. **Fallbacks** — did any agent fallback trigger? Was the fallback successful?
5. **Review logs** — read the raw run artifacts for detailed executor/reviewer output

## Run artifacts

Run artifacts are stored at:
` + "`~/.config/praetor/projects/<project-key>/runtime/<run-id>/`" + `

- ` + "`events.jsonl`" + ` — full event stream
- ` + "`diagnostics/performance.jsonl`" + ` — performance metrics
- ` + "`snapshot.json`" + ` — state snapshot at end of run

## Allowed tools

Bash(praetor plan list), Bash(praetor plan status *), Bash(praetor plan diagnose *), Bash(praetor plan eval *), Bash(praetor eval), Read, Glob
`
