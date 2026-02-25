# Loop orchestration

`praetor loop` is the plan-driven orchestration runtime. It replaces the bash loop scripts with a compiled Go binary that provides git safety, cost tracking, crash recovery, and structured observability.

## Commands

### Create a plan

```bash
praetor loop plan new my-feature
```

Generates a skeleton plan file at `docs/plans/PLAN-PRAETOR-<date>-my-feature.json` with two sample tasks.

### Run a plan

```bash
praetor loop run --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

### Run with options

```bash
praetor loop run \
  --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json \
  --default-executor claude \
  --default-reviewer claude \
  --max-retries 5 \
  --post-task-hook ./scripts/validate.sh \
  --tmux-session praetor-team \
  --timeout 2h
```

### Check progress

```bash
praetor loop plan status --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

Output:

```
Plan:     /abs/path/to/plan.json
State:    /home/user/.praetor/projects/abc123/state/plan.state.json
Updated:  2026-02-25T14:30:00Z
Progress: 3/5 tasks done
Status:   in progress

  [x] TASK-001: Implement feature
  [x] TASK-002: Add tests
  [x] TASK-003: Update docs
  [ ] TASK-004: Integration tests
  [ ] TASK-005: Final review
```

### List all tracked plans

```bash
praetor loop plan list
```

### Reset plan state

```bash
praetor loop plan reset --plan docs/plans/PLAN-PRAETOR-2026-02-25-my-feature.json
```

Removes state, lock, retry counters, and feedback files for the plan. Does not delete the plan file itself.

## Plan format

Plans are JSON files with a `tasks` array. Each task can specify its own executor, reviewer, and model:

```json
{
  "$schema": "../schemas/loop-plan.schema.json",
  "title": "implement user auth",
  "tasks": [
    {
      "id": "TASK-001",
      "title": "Add password hashing",
      "executor": "codex",
      "reviewer": "claude",
      "model": "sonnet",
      "description": "Implement bcrypt password hashing in pkg/auth.",
      "criteria": "All existing tests pass. New tests cover hash and verify."
    },
    {
      "id": "TASK-002",
      "title": "Add login endpoint",
      "depends_on": ["TASK-001"],
      "executor": "codex",
      "reviewer": "claude",
      "description": "Add POST /login endpoint using the auth package.",
      "criteria": "Endpoint returns 200 with valid credentials, 401 otherwise."
    }
  ]
}
```

### Task fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | no | Unique task identifier. Auto-generated as `auto-<index>` if omitted. |
| `title` | yes | Short task description. |
| `depends_on` | no | Array of task IDs that must complete before this task runs. |
| `executor` | no | Agent for execution: `codex` or `claude`. Falls back to `--default-executor`. |
| `reviewer` | no | Agent for review: `codex`, `claude`, or `none`. Falls back to `--default-reviewer`. |
| `model` | no | Model hint: `sonnet`, `opus`, or `haiku`. |
| `description` | no | Detailed task description included in the executor prompt. |
| `criteria` | no | Acceptance criteria included in both executor and reviewer prompts. |

A JSON Schema is available at `docs/schemas/loop-plan.schema.json` for editor validation.

### Validation rules

The plan is validated at load time:

- `tasks` array cannot be empty.
- Every task must have a non-empty `title`.
- Task IDs must be unique.
- `depends_on` must reference existing task IDs.
- `executor` must be `codex` or `claude` (if specified).
- `reviewer` must be `codex`, `claude`, or `none` (if specified).
- `model` must be `sonnet`, `opus`, or `haiku` (if specified).

## Runtime model

### State isolation

All mutable state is stored under `~/.praetor/projects/<project-hash>/`:

```text
~/.praetor/projects/<hash>/
├── state/          # Task state per plan (.state.json)
├── locks/          # PID-based run locks
├── logs/           # Per-run execution logs
│   └── <timestamp>-<task>-<sig>/
│       ├── executor.system.txt      # Executor system prompt
│       ├── executor.prompt.txt      # Executor task prompt
│       ├── executor.output.txt      # Executor output
│       ├── executor.prompt          # Raw prompt file (tmux mode)
│       ├── executor.system-prompt   # Raw system prompt file (tmux mode)
│       ├── executor.stdout          # Raw stdout capture (tmux mode)
│       ├── executor.stderr          # Raw stderr capture (tmux mode)
│       ├── executor.exit            # Exit code (tmux mode)
│       ├── executor.run.sh          # Wrapper script (tmux mode)
│       ├── reviewer.*               # Same structure for reviewer
│       ├── post-hook.stdout         # Post-task hook stdout (if used)
│       └── post-hook.stderr         # Post-task hook stderr (if used)
├── retries/        # Retry counters per task signature (.count)
├── feedback/       # Reviewer feedback per task signature (.txt)
├── snapshots/      # Git HEAD snapshots for rollback (.sha)
├── costs/          # Cost tracking ledger (tracking.tsv)
└── checkpoints/    # Audit log (history.tsv) and current state (.state)
```

The project hash is derived from the git repository root path (SHA-256), ensuring state isolation between projects.

### Task lifecycle

```
open ──► executor ──► post-task hook ──► reviewer ──► done
              │              │                │
              ▼              ▼                ▼
         fail/crash     hook failed     review rejected
              │              │                │
              └──────────────┴────────────────┘
                             │
                        retry (with feedback)
                             │
                     ┌───────┴───────┐
                     ▼               ▼
               retry limit      next attempt
               reached          (back to executor)
                     │
                     ▼
                   stuck
```

### Dependency resolution

Tasks are selected in order. A task is runnable when:

1. Its status is `open`.
2. All tasks in its `depends_on` list have status `done`.

The runner picks the first runnable task. If no task is runnable and open tasks remain, the plan is blocked.

### Locking

Each plan run acquires a PID-based lock file. If the lock holder process is still alive, a new run is rejected unless `--force` is used. Locks are released on exit.

### Retry mechanism

Each task has a retry counter identified by a SHA-256 signature of its key (ID or index+title). On failure:

1. Retry counter increments.
2. Feedback from the reviewer (or hook, or crash message) is persisted.
3. On the next attempt, the feedback is injected into the executor prompt with emphatic formatting.
4. When retry count reaches `--max-retries`, the task is stuck and the run stops.

Retry counters and feedback are cleared when a task completes successfully. They persist across process restarts.

## Safety mechanisms

### Git safety

Enabled by default (`--git-safety`). Before each executor run:

1. `git rev-parse HEAD` is saved as `snapshots/<runID>.sha`.
2. On any failure (executor crash, self-reported FAIL, hook failure, reviewer rejection): `git reset --hard <sha>` + `git clean -fd` restores the working tree.
3. On success: the snapshot file is discarded.

Disable with `--git-safety=false` for non-git workspaces.

### Post-task hook

A custom script (`--post-task-hook <path>`) runs between the executor and reviewer phases:

- The hook runs with the workdir as CWD.
- Exit code 0: proceed to reviewer.
- Exit code non-zero: increment retry, store last 50 lines of stdout as feedback, rollback git if enabled.
- Stdout/stderr are saved to `<runDir>/post-hook.stdout` and `post-hook.stderr`.

Use this for linters, type checkers, or integration tests that must pass before review.

### Pre-flight checks

Before the loop starts, all required binaries are validated with `exec.LookPath`:

- `tmux` (always required)
- `codex` (if any task uses the codex executor or reviewer)
- `claude` (if any task uses the claude executor or reviewer)

Missing binaries produce a clear error listing all that are absent.

## Observability

### Cost tracking

Every agent invocation records a cost entry to `costs/tracking.tsv`:

```
timestamp	run_id	task_id	agent	role	duration_s	status	cost_usd
2026-02-25T14:30:00Z	20260225-143000-TASK-001-abc12345	TASK-001	codex	executor	45.20	pass	0.032100
```

Cost is extracted from:
- **Claude**: `ResultMessage.TotalCostUSD` from the stream-json protocol.
- **Codex**: `total_cost_usd` from `--json` output (tmux mode).

The run summary displays accumulated cost:

```
Run summary  done=5 rejected=1 iterations=6 cost=$0.2341 duration=2m15s
```

### Checkpoint audit log

Every state transition appends to `checkpoints/history.tsv`:

```
timestamp	status	task_id	signature	run_id	message
```

Tracked transitions: `completed`, `executor_crashed`, `executor_self_fail`, `hook_failed`, `reviewer_crashed`, `review_rejected`, `blocked`.

The current checkpoint is also written as a key-value file at `checkpoints/<plan>.state`.

### Terminal output

Colored, structured output shows real-time progress:

```
=== Praetor Loop ===
Plan:        implement user auth
Plan file:   docs/plans/plan.json
State:       ~/.praetor/projects/abc123/state/plan.state.json
Progress:    0/2 done
tmux:        praetor-abc123

[1/2] TASK-001 Add password hashing
  executor (codex) attempt 1/3 [45.2s]
  hook     (post-task) ./scripts/validate.sh
  reviewer (claude) review complete [12.1s]
  [ok] Completed: TASK-001

[2/2] TASK-002 Add login endpoint
  executor (codex) attempt 1/3 [38.7s]
  reviewer (claude) review complete [8.3s]
  [ok] Completed: TASK-002

Run summary  done=2 rejected=0 iterations=2 cost=$0.0891 duration=1m44s
```

Disable colors with `--no-color` or the `NO_COLOR` environment variable.

## Tmux execution

Every agent invocation runs in a dedicated tmux window:

1. A wrapper shell script is generated with the agent command, I/O redirection, and a `tmux wait-for` signal.
2. The script is launched in a new tmux window named `praetor-<task>-<role>`.
3. The runner blocks on `tmux wait-for <channel>` until the script completes.
4. Exit code, stdout, and stderr are read from files.

The tmux session is auto-created if it doesn't exist and auto-destroyed on clean exit (only if praetor created it). Attach to the session to watch agents work in real time:

```bash
tmux attach -t praetor-<project-hash>
```
