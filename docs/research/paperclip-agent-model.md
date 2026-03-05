# Paperclip Agent Model: Heartbeat Protocol, Adapters, and Execution Lifecycle

Research output for TASK-002. Source: https://github.com/paperclipai/paperclip

---

## 1. The Heartbeat Protocol

Agents in Paperclip do not run continuously. They run in **heartbeats**: short, discrete execution windows triggered by a **wakeup**. Each heartbeat spawns the configured adapter, gives it a prompt and context, lets it work until it exits/times out/is cancelled, then stores results and updates the UI.

The heartbeat is the core scheduling primitive. It answers: "when does an agent run, and what happens when multiple triggers overlap?"

### 1.1 Wakeup Triggers (4 sources, not 5)

All wakeup sources flow through a single entrypoint: `enqueueWakeup()` in `server/src/services/heartbeat.ts`. No source invokes adapters directly.

| Source | Description | triggerDetail |
|--------|-------------|---------------|
| `timer` | Periodic heartbeat per agent, based on `intervalSec` in heartbeat policy. Checked by a scheduler worker loop. | `system` |
| `assignment` | Issue assigned/reassigned to agent. Fired by issue mutation service. | `manual` / `ping` |
| `on_demand` | Manual wakeup via UI button or API `POST /agents/:agentId/wakeup`. | `manual` / `ping` |
| `automation` | System-triggered wakeup for callbacks, external automations, or internal events (e.g., comment mentions, approval status changes). | `callback` / `system` |

The spec originally listed 5 triggers, but the implementation consolidates to 4 sources with sub-detail via `triggerDetail`. Assignment wakes are a specialized `on_demand` or `automation` depending on the call site.

### 1.2 Agent Heartbeat Policy

Each agent has a `runtimeConfig.heartbeat` object that controls scheduling:

```json
{
  "heartbeat": {
    "enabled": true,
    "intervalSec": 300,
    "wakeOnDemand": true,
    "maxConcurrentRuns": 1
  }
}
```

The policy is parsed by `parseHeartbeatPolicy()` which normalizes defaults:
- `enabled`: true (controls timer wakes only)
- `intervalSec`: 0 (no timer until set)
- `wakeOnDemand`: true (controls all non-timer wakes)
- `maxConcurrentRuns`: 1 (clamped 1-10)

Agents in `paused`, `terminated`, or `pending_approval` status reject all wakeups.

---

## 2. Wakeup Coalescing Logic (6 outcomes)

When `enqueueWakeup()` is called, the system determines one of 6 possible outcomes. The logic varies based on whether the wakeup is issue-scoped.

### 2.1 Issue-scoped wakeups (when `issueId` is present)

The system uses an **issue execution lock** (`issues.executionRunId`) to ensure only one agent works on an issue at a time.

| # | Outcome | Condition | What happens |
|---|---------|-----------|--------------|
| 1 | **skipped** | Agent is not invokable (paused/terminated), heartbeat disabled for this source, or issue not found | Wakeup written as `status: "skipped"`, no run created |
| 2 | **coalesced** (same agent) | Issue already has an active run from the **same agent name** | Context snapshot merged into existing run, wakeup written as `status: "coalesced"`, no new run created |
| 3 | **deferred** (different agent) | Issue already has an active run from a **different agent** | Wakeup written as `status: "deferred_issue_execution"`, queued for promotion when the active run finishes |
| 4 | **deferred + coalesced** | Same as #3, but a deferred wakeup for the same issue already exists | Existing deferred wakeup's context is merged, `coalescedCount` incremented |
| 5 | **queued** (no active run) | No active run on this issue | New `heartbeat_runs` row created with `status: "queued"`, issue's `executionRunId` set to this run |
| 6 | **queued + comment followup** | Issue has an active run from same agent but wake is triggered by a comment mention | Instead of coalescing, a new queued run is created to handle the comment after the current run finishes |

### 2.2 Non-issue-scoped wakeups (no `issueId`)

Simpler path with task-scope coalescing:

| # | Outcome | Condition |
|---|---------|-----------|
| 1 | **coalesced** | An existing queued or running run has the same `taskKey` scope | Context merged into existing run |
| 2 | **queued** | No same-scope active run exists | New run created |

### 2.3 Context Merging

When coalescing, `mergeCoalescedContextSnapshot()` spreads the incoming context over the existing one, preserving the latest `commentId` and `wakeCommentId`. This allows the running agent to see updated context without restarting.

### 2.4 Deferred Wake Promotion

When a run finishes, `releaseIssueExecutionAndPromote()` is called. It:
1. Clears the issue's `executionRunId` lock
2. Finds the oldest `deferred_issue_execution` wakeup for that issue
3. Validates the deferred agent is still invokable
4. Creates a new queued run and promotes the deferred wakeup to `status: "queued"`
5. Sets the issue's execution lock to the new run
6. Loops through any remaining deferred requests if the first agent is no longer invokable

---

## 3. Adapter System

### 3.1 The Three-Module Adapter Pattern

Each adapter is a separate npm package under `packages/adapters/<name>/` with three modules consumed by three registries:

```
packages/adapters/<name>/
  src/
    index.ts              # Shared metadata: type key, label, supported models
    server/
      execute.ts          # Core: spawns agent process, captures output
      parse.ts            # Output parsing: stdout -> structured result
      test.ts             # Environment diagnostics: checks if CLI is installed
    ui/
      parse-stdout.ts     # Stdout line -> TranscriptEntry[] for run viewer
      build-config.ts     # Form values -> adapterConfig JSON
    cli/
      format-event.ts     # Terminal output for `paperclipai run --watch`
```

| Registry | Location | What it consumes |
|----------|----------|------------------|
| **Server** (`server/src/adapters/registry.ts`) | Server | `execute()`, `testEnvironment()`, `sessionCodec`, `models` |
| **UI** (`ui/src/adapters/`) | React SPA | `parseStdoutLine()`, `buildConfig()` |
| **CLI** (`cli/src/adapters/`) | CLI tool | `formatStdoutEvent()` |

### 3.2 Server Adapter Interface

All server adapters implement `ServerAdapterModule`:

```ts
interface ServerAdapterModule {
  type: string;                                                    // e.g., "claude_local"
  execute(ctx: AdapterExecutionContext): Promise<AdapterExecutionResult>;
  testEnvironment(ctx: AdapterEnvironmentTestContext): Promise<AdapterEnvironmentTestResult>;
  sessionCodec?: AdapterSessionCodec;                              // serialize/deserialize session state
  supportsLocalAgentJwt?: boolean;                                 // inject PAPERCLIP_API_KEY
  models?: AdapterModel[];                                         // static model list
  listModels?: () => Promise<AdapterModel[]>;                      // dynamic model discovery
  agentConfigurationDoc?: string;                                  // human-readable config docs
}
```

The `AdapterExecutionContext` provides:
- `runId`, `agent`, `runtime` (session state), `config` (adapter config), `context` (wakeup context)
- `onLog(stream, chunk)` callback for streaming stdout/stderr to RunLogStore + WebSocket
- `onMeta(meta)` callback for recording invocation details (command, args, cwd, env)
- `authToken` for injecting `PAPERCLIP_API_KEY`

The `AdapterExecutionResult` returns:
- `exitCode`, `signal`, `timedOut`, `errorMessage`, `errorCode`
- `usage` (inputTokens, outputTokens, cachedInputTokens)
- `sessionId`, `sessionParams`, `sessionDisplayId` (for session persistence)
- `provider`, `model`, `billingType`, `costUsd`
- `resultJson`, `summary`
- `clearSession` (force session reset, e.g., on max turns)

### 3.3 All 7 Adapters Cataloged

The codebase has **7 adapters** (not 6 as stated in the task), registered in `server/src/adapters/registry.ts`:

#### Local CLI Adapters (process-based, resumable sessions)

| # | Adapter | Type Key | Execution Mode | Session Resume | Local Agent JWT |
|---|---------|----------|----------------|----------------|-----------------|
| 1 | **Claude Local** | `claude_local` | `claude --print - --output-format stream-json --verbose` | `--resume <sessionId>` | Yes |
| 2 | **Codex Local** | `codex_local` | `codex exec --json <prompt>` | `codex exec --json resume <sessionId> <prompt>` | Yes |
| 3 | **Cursor Local** | `cursor` | Cursor CLI invocation | Session-based | Yes |
| 4 | **OpenCode Local** | `opencode_local` | OpenCode CLI invocation | Session-based | Yes |
| 5 | **OpenClaw** | `openclaw` | OpenClaw HTTP adapter | No local JWT | No |

#### Generic Adapters (built into server)

| # | Adapter | Type Key | Execution Mode | Session Resume | Local Agent JWT |
|---|---------|----------|----------------|----------------|-----------------|
| 6 | **Process** | `process` | `spawn(command, args)` arbitrary shell command | No | No |
| 7 | **HTTP** | `http` | `fetch(url, { method, body })` webhook | No | No |

#### Claude Local - Detailed Execution Flow

1. Build runtime config: resolve `cwd` (workspace > config > process.cwd), build env vars (PAPERCLIP_* environment)
2. Build skills directory: create tmpdir with symlinked `.claude/skills/` from repo's `skills/` dir
3. Render prompt template with `{{agent.id}}`, `{{company.id}}`, `{{run.id}}`, etc.
4. Build CLI args: `--print - --output-format stream-json --verbose` + optional `--resume`, `--model`, `--max-turns`, `--dangerously-skip-permissions`, `--append-system-prompt-file`, `--add-dir`
5. Spawn child process via `runChildProcess()` with stdin=prompt, streaming stdout/stderr to `onLog`
6. Parse stream-json output: extract `session_id`, `model`, `usage`, `costUsd`, `summary` from NDJSON events
7. On session resume failure (unknown session error): automatically retry with fresh session
8. Return structured `AdapterExecutionResult` with session params including `cwd` and `workspaceId` for workspace-aware resume

#### Process Adapter - Minimal

Spawns arbitrary shell command with Paperclip env vars. No output parsing, no session support. Returns raw stdout/stderr.

#### HTTP Adapter - Fire-and-forget

Sends POST request with `{ agentId, runId, context }` body to configured URL. No streaming, no session support. Success = HTTP 2xx.

---

## 4. Run Lifecycle Pipeline

### 4.1 Complete Pipeline: Trigger -> Enqueue -> Claim -> Execute -> Finalize

```
Trigger arrives (timer/assignment/on_demand/automation)
    |
    v
enqueueWakeup()
    |-- Policy check: agent invokable? source enabled?
    |-- Issue execution lock: coalesce/defer/queue?
    |-- Non-issue: task-scope coalesce or queue?
    |
    v (if queued)
Create heartbeat_runs row (status: "queued")
Create agentWakeupRequests row (status: "queued")
Publish "heartbeat.run.queued" live event
    |
    v
startNextQueuedRunForAgent()
    |-- withAgentStartLock() -- serializes starts per agent
    |-- Check maxConcurrentRuns vs running count
    |-- Claim oldest queued runs up to available slots
    |
    v
claimQueuedRun()
    |-- Atomic UPDATE with WHERE status='queued' (prevents double-claim)
    |-- Set status: "running", startedAt
    |-- Publish "heartbeat.run.status" live event
    |-- Set wakeup request to "claimed"
    |
    v
executeRun()
    |-- Resolve session state (task-scoped or agent-level)
    |-- Resolve workspace (project primary > task session > agent home)
    |-- Resolve adapter config (merge issue assignee overrides)
    |-- Set agent status to "running"
    |-- Begin RunLogStore handle
    |-- Create auth JWT if adapter supports it
    |-- Call adapter.execute() with context + callbacks
    |-- Parse result: determine outcome (succeeded/failed/cancelled/timed_out)
    |-- Finalize RunLogStore
    |-- Update heartbeat_runs with result, usage, session, logs
    |-- Update wakeup request status
    |-- Update agentRuntimeState (accumulate tokens, costs)
    |-- Persist task session (upsert or clear)
    |-- Record cost event
    |-- Release issue execution lock
    |-- Promote deferred wakeups
    |
    v
finalizeAgentStatus()
    |-- If other runs still running: stay "running"
    |-- If succeeded/cancelled: set "idle"
    |-- If failed/timed_out: set "error"
    |-- Publish "agent.status" live event
    |
    v
startNextQueuedRunForAgent() (in finally block)
    |-- Pick up next queued run if slots available
```

### 4.2 Concurrency Control

- **Per-agent start lock** (`withAgentStartLock`): serializes the claim+start sequence per agent using a promise chain stored in `startLocksByAgent` map. Prevents race conditions where multiple triggers could claim runs simultaneously.
- **Atomic claim** (`claimQueuedRun`): uses `UPDATE ... WHERE status='queued'` to ensure only one claimer wins.
- **Max concurrent runs**: defaults to 1, configurable up to 10. Checked before claiming.
- **Issue execution lock** (`issues.executionRunId`): row-level `FOR UPDATE` lock prevents concurrent work on the same issue across agents.

### 4.3 Error Handling and Recovery

**During execution:**
- If adapter throws, run is marked `failed` with captured error text
- Timeout: SIGTERM, then SIGKILL after `graceSec`
- Cancellation: detected by checking run status after adapter returns

**On server restart:**
- `reapOrphanedRuns()` finds `queued`/`running` runs with no matching entry in `runningProcesses` map
- Marks them as `failed` with `errorCode: "process_lost"`
- Releases issue execution locks and promotes deferred wakeups
- Applies staleness threshold to avoid false positives

### 4.4 Run Status States

```
queued -> running -> succeeded
                  -> failed
                  -> timed_out
                  -> cancelled
```

Wakeup request statuses: `queued`, `claimed`, `coalesced`, `skipped`, `deferred_issue_execution`, `completed`, `failed`, `cancelled`.

---

## 5. Session Persistence Mechanism

### 5.1 Two-Level Session Model

Paperclip maintains session state at two levels:

#### Level 1: Agent-level (`agent_runtime_state` table)

One row per agent. Stores:
- `session_id`: legacy single session ID
- `last_run_id`, `last_run_status`: most recent run reference
- `total_input_tokens`, `total_output_tokens`, `total_cached_input_tokens`, `total_cost_cents`: cumulative usage
- `last_error`: most recent error

#### Level 2: Task-scoped (`agent_task_sessions` table)

One row per `(company_id, agent_id, adapter_type, task_key)`. Stores:
- `session_params_json`: adapter-defined shape (e.g., `{ sessionId, cwd, workspaceId, repoUrl }`)
- `session_display_id`: human-readable session identifier
- `last_run_id`, `last_error`: most recent run for this task scope

### 5.2 Task Key Derivation

The `taskKey` is derived from wakeup context in priority order:
1. `contextSnapshot.taskKey`
2. `contextSnapshot.taskId`
3. `contextSnapshot.issueId`
4. `payload.taskKey` / `payload.taskId` / `payload.issueId`

This means different issues get separate sessions, while wakeups for the same issue share a session.

### 5.3 Session Lifecycle per Heartbeat Run

**Before execution:**
1. Derive `taskKey` from context
2. If `taskKey` exists, look up `agent_task_sessions` for this `(agent, adapter, taskKey)`
3. Deserialize session params through adapter's `sessionCodec`
4. Determine if session should be reset (timer wake, manual invoke, or issue assignment -> fresh start)
5. Resolve workspace and validate session cwd matches current workspace
6. Pass session state to adapter via `runtime.sessionParams`

**During execution:**
- Claude adapter uses `--resume <sessionId>` to resume previous conversation
- If resume fails (unknown session error), adapter retries once with fresh session and sets `clearSession: true`

**After execution:**
1. `resolveNextSessionState()` processes adapter result:
   - If `clearSession`: null out all session state
   - If explicit `sessionParams` returned: use those
   - If explicit `sessionId` returned: wrap in params object
   - Otherwise: keep previous params
2. Serialize through adapter's `sessionCodec`
3. Upsert `agent_task_sessions` row with new params
4. Update `agent_runtime_state` with latest legacy session ID

### 5.4 Session Codec Pattern

Each adapter provides an `AdapterSessionCodec`:

```ts
interface AdapterSessionCodec {
  deserialize(raw: unknown): Record<string, unknown> | null;  // DB -> adapter
  serialize(params: Record<string, unknown> | null): Record<string, unknown> | null;  // adapter -> DB
  getDisplayId?: (params: Record<string, unknown> | null) => string | null;  // human label
}
```

The codec allows each adapter to define its own session state shape while the heartbeat service handles persistence generically.

### 5.5 Session Reset Triggers

`shouldResetTaskSessionForWake()` returns true when:
- `wakeReason === "issue_assigned"` (fresh task assignment = clean slate)
- `wakeSource === "timer"` (periodic heartbeat = let agent decide from scratch)
- `wakeSource === "on_demand" && triggerDetail === "manual"` (human clicked "run" = intentional fresh start)

### 5.6 Workspace-Aware Session Resume

Claude Local stores `cwd` and `workspaceId` in session params. On resume:
- If the previous session's `cwd` matches the current resolved workspace: resume normally
- If they differ: skip resume, log a warning, start fresh
- If agent was using a fallback workspace (agent home) but a project workspace is now available: migrate session params to new workspace

---

## 6. Key Architectural Insights for Praetor

### 6.1 What Praetor Can Learn

1. **Centralized wakeup coordination**: Paperclip's single `enqueueWakeup()` entrypoint with policy checks, coalescing, and issue execution locking is cleaner than having triggers directly invoke adapters. Praetor's pipeline runner could benefit from a similar single-entry scheduling layer.

2. **Task-scoped session persistence**: The `(agent, adapter, taskKey)` session model enables parallel task work without session collision. Praetor's worktree model serves a similar purpose but at the filesystem level rather than the session level.

3. **Adapter session codec**: Each adapter owning its session serialization format is elegant. Praetor's adapters could benefit from this pattern instead of assuming a uniform session shape.

4. **Issue execution lock with deferred promotion**: The single-assignee model with deferred wake queuing prevents concurrent work and automatically chains agents. This is more sophisticated than Praetor's current approach.

5. **Graceful session retry**: Claude adapter's automatic retry on unknown session error (try resume -> fail -> retry fresh) is robust and avoids stuck agents.

### 6.2 Key Differences

| Aspect | Paperclip | Praetor |
|--------|-----------|---------|
| Scheduling | Wakeup-driven (heartbeat) | Plan-driven (sequential pipeline) |
| Concurrency | Issue execution lock (DB row lock) | Git worktree isolation |
| Session | DB-persisted per task scope | Implicit via agent CLI session |
| Agent communication | Env vars + REST API | System prompt + CLI args |
| Adapter pattern | 3-module (server/UI/CLI) | Single module (execute only) |
| Task model | Issue board (Kanban) | Ordered plan (sequential) |
