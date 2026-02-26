# Task orchestration

Praetor's core is plan-driven task orchestration. Define a sequence of tasks in a JSON plan, then execute it with `praetor run`. Each task goes through an executor agent, an optional post-task hook, and a reviewer agent before being marked as done.

## Commands

### Create a plan

```bash
praetor plan create my-feature
```

Generates a skeleton plan at `docs/plans/PLAN-PRAETOR-<date>-my-feature.json` with two sample tasks.

### Run a plan

```bash
praetor run docs/plans/my-plan.json
```

### Run with options

```bash
praetor run docs/plans/my-plan.json \
  --executor claude \
  --reviewer claude \
  --max-retries 5 \
  --hook ./scripts/validate.sh \
  --tmux-session praetor-team \
  --timeout 2h
```

### Check progress

```bash
praetor plan status docs/plans/my-plan.json
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
  [!] TASK-005: Final review (failed, attempt 3)
```

Status markers: `[x]` done, `[ ]` pending, `[>]` executing/reviewing, `[!]` failed.

### List all tracked plans

```bash
praetor plan list
```

### Reset plan state

```bash
praetor plan reset docs/plans/my-plan.json
```

Removes state, lock, and legacy retry/feedback files for the plan. Does not delete the plan file itself.

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
| `executor` | no | Agent for execution: `codex` or `claude`. Falls back to `--executor`. |
| `reviewer` | no | Agent for review: `codex`, `claude`, or `none`. Falls back to `--reviewer`. |
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
├── retries/        # Legacy retry counters (migrated into state file on load)
├── feedback/       # Legacy feedback files (migrated into state file on load)
├── costs/          # Cost tracking ledger (tracking.tsv)
└── checkpoints/    # Audit log (history.tsv) and current state (.state)
```

The project hash is derived from the git repository root path (SHA-256), ensuring state isolation between projects.

### Task state machine

Each task follows an explicit finite state machine with five states and validated transitions. The design adapts Rob Pike's function-as-state pattern for persistence: each state maps to a step function that does its work and returns the next `TaskStatus` to persist.

#### States

| Status | Description | Terminal |
|--------|-------------|----------|
| `pending` | Ready to execute (or awaiting retry). | no |
| `executing` | Executor agent is running. | no |
| `reviewing` | Reviewer agent is evaluating the result. | no |
| `done` | Task completed and merged successfully. | yes |
| `failed` | All retry attempts exhausted. | yes |

#### State diagram

```
┌───────────┐
│  pending   │◀─────────────────────────────────┐
└─────┬──────┘                                  │
      │ stepExecute                             │
      ▼                                         │
┌───────────┐   crash/FAIL     ┌─────────┐     │
│ executing  │────────────────▶│  guard   │─────┘  attempt < max
└─────┬──────┘                 │retry ok? │
      │ PASS                   └────┬─────┘
      ▼                             │ attempt >= max
┌───────────┐   reject             ▼
│ reviewing  │──────────┐    ┌──────────┐
└─────┬──────┘          │    │  failed   │ (terminal)
      │ approve         │    └──────────┘
      ▼                 │
┌───────────┐           │
│   done     │           │
└───────────┘           │
      ▲                 │
      └─────────────────┘
        (retry → pending)
```

Tasks with `reviewer = none` skip the `reviewing` state: `executing` transitions directly to `done` on success.

#### Transition table

Every state change is validated against a declarative transition table. Invalid transitions return an error immediately.

```go
var validTransitions = map[TaskStatus][]TaskStatus{
    TaskPending:   {TaskExecuting, TaskFailed},
    TaskExecuting: {TaskReviewing, TaskDone, TaskPending, TaskFailed},
    TaskReviewing: {TaskDone, TaskPending, TaskFailed},
    TaskDone:      {},  // terminal
    TaskFailed:    {},  // terminal
}
```

#### Step functions

| Step | Trigger | Outcomes |
|------|---------|----------|
| **stepExecute** | Task is `pending` | `executing` → run executor → PASS: `reviewing` (or `done` if no reviewer) / FAIL: `pending` (retry) / crash: `pending` (retry) |
| **stepReview** | Task is `reviewing` | Run reviewer → PASS: `done` / FAIL: `pending` (retry) / crash: `pending` (retry) |
| **retryGuard** | Before each step | If `attempt >= maxRetries`: `pending` → `failed` |

#### Crash recovery

On state file load, transient states are reset for crash safety:

- `executing` → `pending` (executor was interrupted)
- `reviewing` → `pending` (reviewer was interrupted)

This is safe because no partial work is committed to the main branch until a task reaches `done`.

#### Legacy migration

State files using the old `"open"` status are transparently migrated to `"pending"` on load. Retry counts from external files (`retries/*.count`) are absorbed into `StateTask.Attempt`. After migration, the state file is the single source of truth.

#### StateTask fields

```go
type StateTask struct {
    // ... plan fields (ID, Title, DependsOn, etc.) ...
    Status   TaskStatus `json:"status"`              // current state
    Attempt  int        `json:"attempt,omitempty"`    // retry count (0 = never tried)
    Feedback string     `json:"feedback,omitempty"`   // last failure feedback
}
```

`Attempt` and `Feedback` are embedded in the state file — no external retry/feedback files are needed in the hot path.

### Dependency resolution

Tasks are selected in order. A task is runnable when:

1. Its status is `pending`.
2. All tasks in its `depends_on` list have status `done`.

The runner picks the first runnable task. If no task is runnable and active (non-terminal) tasks remain, the plan is blocked.

### Locking

Each plan run acquires a PID-based lock file. If the lock holder process is still alive, a new run is rejected unless `--force` is used. Locks are released on exit.

### Retry mechanism

Each task carries its retry state in `StateTask.Attempt` and `StateTask.Feedback`. On failure:

1. `Attempt` increments and `Feedback` stores the failure reason (crash message, reviewer rejection, hook output).
2. The task transitions back to `pending` via a validated state transition.
3. On the next attempt, the feedback is injected into the executor prompt with emphatic formatting.
4. Before each step, a retry guard checks `Attempt >= maxRetries`. If exhausted, the task transitions to `failed` (terminal).

`Attempt` and `Feedback` are cleared when a task completes successfully. They persist across process restarts as part of the state file.

## Safety mechanisms

### Worktree isolation

Enabled by default (`--isolation worktree`). Before each executor run:

1. A dedicated `git worktree` is created on a new branch (`praetor/<task>--<runID>`).
2. The executor and reviewer agents operate inside the worktree, never touching the main working tree.
3. On success: uncommitted changes are auto-committed, the branch is merged into main, and the worktree is removed.
4. On any failure (executor crash, self-reported FAIL, hook failure, reviewer rejection): the worktree and branch are deleted without merging — the main tree stays untouched.

Disable with `--isolation off` for non-git workspaces. Orphan worktree metadata from previous crashes is pruned automatically at startup via `git worktree prune`.

### Post-task hook

A custom script (`--hook <path>`) runs between the executor and reviewer phases:

- The hook runs with the workdir as CWD.
- Exit code 0: proceed to reviewer.
- Exit code non-zero: increment retry, store last 50 lines of stdout as feedback, discard the worktree.
- Stdout/stderr are saved to `<runDir>/post-hook.stdout` and `post-hook.stderr`.

Use this for linters, type checkers, or integration tests that must pass before review.

### Pre-flight checks

Before the run starts, all required binaries are validated with `exec.LookPath`:

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
Isolation:   worktree
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
