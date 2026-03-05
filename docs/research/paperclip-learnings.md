# Key Learnings from Paperclip Applicable to Praetor

Research output for TASK-006. Distills actionable learnings from TASK-001 through TASK-005.

---

## 1. Session Persistence for Task-Scoped Agent State

**Paperclip approach:** Persists agent session state keyed on `(agentId, adapterType, taskKey)` in the `agent_task_sessions` table. Each adapter provides a `SessionCodec` for serialization. Claude adapter stores `{ sessionId, cwd, workspaceId }` enabling `--resume` across heartbeats. Session reset triggers are explicit: fresh assignment, timer wake, or manual invocation.

**Relevance to Praetor:** Every Praetor invocation is currently stateless -- agents start fresh each time, even on retries of the same task. This wastes context window capacity re-establishing state the agent already had. Claude Code's `--resume` flag exists but Praetor never uses it.

**Adaptation:** Implement as filesystem-backed session store: `state/<plan>/<task-id>/session.<adapter>.json`. Each adapter's `Agent` interface gains an optional `SessionCodec` for provider-specific state. On retry, the pipeline loads the previous session and passes it to the adapter (e.g., `--resume <sessionId>` for Claude). Session reset on task re-assignment or explicit `--fresh` flag. No database required -- one JSON file per task-adapter pair.

**Applicability:** High. Directly reduces retry cost and improves agent continuity. Respects file-based state model.

---

## 2. Adapter Session Codec Pattern

**Paperclip approach:** Each adapter owns its session serialization format via an `AdapterSessionCodec` interface with `serialize()`, `deserialize()`, and `getDisplayId()`. The heartbeat service handles persistence generically while each adapter defines its own state shape.

**Relevance to Praetor:** Praetor's `Agent` interface is uniform (`Plan()`, `Execute()`, `Review()`), which works well for invocation but doesn't accommodate provider-specific session state. Claude needs `sessionId`, Codex needs `sessionId` with different semantics, REST providers need nothing.

**Adaptation:** Add an optional `SessionState` interface to the `Agent` abstraction with `MarshalSession() ([]byte, error)` and `UnmarshalSession([]byte) error`. Adapters that don't support sessions return nil. The pipeline persists whatever the adapter returns, without understanding its contents. Idiomatic Go pattern -- zero new dependencies.

**Applicability:** High. Natural extension of the existing adapter pattern. Required foundation for Learning #1.

---

## 3. Graceful Session Retry on Resume Failure

**Paperclip approach:** The Claude adapter catches "unknown session" errors when attempting `--resume`. On failure, it automatically retries with a fresh session and sets `clearSession: true` so the old session is discarded. This prevents agents from getting stuck on stale session references.

**Relevance to Praetor:** If Praetor adopts session persistence (Learning #1), stale sessions become a risk -- sessions may expire, agent CLIs may be updated, or workspace state may have drifted. Without graceful retry, a stale session would cause a hard failure.

**Adaptation:** In the adapter's `Execute()` method, catch session-resume errors (exit code + stderr pattern matching). Retry once without the session flag. If the retry succeeds, clear the stored session file. This is adapter-internal logic -- the pipeline doesn't need to know about it. Add a `session_retry` event type for observability.

**Applicability:** High. Essential companion to session persistence. Prevents a class of silent failures.

---

## 4. Cumulative Agent Statistics Across Runs

**Paperclip approach:** The `agent_runtime_state` table accumulates per-agent lifetime statistics: `total_input_tokens`, `total_output_tokens`, `total_cached_input_tokens`, `total_cost_cents`, `last_run_id`, `last_run_status`, `last_error`. Updated atomically after each heartbeat run.

**Relevance to Praetor:** Cost data is currently scattered across per-run `costs/` files and `runtime/<run-id>/diagnostics/performance.jsonl`. There is no aggregate view of how much a given provider has cost across all plan runs, or which provider is most efficient. The `plan diagnose --query costs` command shows per-run data only.

**Adaptation:** Add a `stats/<agent-name>.json` file under the project directory. After each task execution, append token counts and cost data. Structure: `{ "total_input_tokens": N, "total_output_tokens": N, "total_cost_cents": N, "total_runs": N, "last_run_id": "...", "last_error": "..." }`. The `praetor eval` command can read these for cross-run analytics. File-based, no schema migrations, backward-compatible.

**Applicability:** High. Low effort, high value for cost visibility and provider comparison.

---

## 5. Monetary Budget Tracking with Auto-Stop

**Paperclip approach:** Two-tier budget system (company + agent) with `budgetMonthlyCents` and `spentMonthlyCents` counters. Cost events atomically increment spent counters. When an agent's spend reaches 100% of budget, the agent is auto-paused. 80% is a soft alert. Budget tracked per-provider and per-model for analytics.

**Relevance to Praetor:** Praetor tracks context-char budgets for prompt sizing but has no monetary cost tracking. As plans grow larger (multi-task, multi-retry), actual LLM spending can be significant and unpredictable. There is no ceiling to prevent runaway costs.

**Adaptation:** Add optional `budget_cents` to `plan.settings.execution_policy`. During execution, the pipeline accumulates `spent_cents` from adapter-reported cost data (already available via `AgentResponse.Usage`). When `spent_cents >= budget_cents`, emit a `budget_exceeded` event and halt the run with `RunOutcome: canceled`. Simpler than Paperclip's 2-tier model -- one ceiling per plan run. Store cumulative spend in the snapshot for recovery. No new dependencies.

**Applicability:** High. Critical safety mechanism for production use. Minimal implementation effort.

---

## 6. Activity Logging with Actor Types

**Paperclip approach:** The `activity_log` table records every mutation with `actorType` (agent/user/system), `actorId`, `action` (dot-notation like `issue.created`), `entityType`, `entityId`, and structured `details`. Indexed by `(companyId, createdAt)`, `(runId)`, `(entityType, entityId)` for forensic queries.

**Relevance to Praetor:** Praetor's JSONL event files capture execution events (`agent_start`, `agent_complete`, `gate_result`, etc.) but lack mutation provenance. When a task state changes, the event records what happened but not who/what caused it (executor output? reviewer decision? stall detection? pipeline gate?). This makes debugging multi-retry scenarios harder.

**Adaptation:** Extend the existing event schema with `actor_type` (executor/reviewer/pipeline/gate/user) and `entity_type` (task/plan/session) fields. No new storage -- enrich the existing `events.jsonl` format. Bump `schema_version` to 2. The `plan diagnose` command gains richer attribution in its output. Backward-compatible: v1 events without actor fields are treated as `actor_type: "pipeline"`.

**Applicability:** Medium. Improves debuggability. Low implementation effort since it extends existing infrastructure.

---

## 7. Task Execution Lock with Deferred Promotion

**Paperclip approach:** The `issues.executionRunId` column acts as a database-level lock ensuring only one agent works on an issue at a time. When a wakeup arrives for a locked issue, it is deferred (`deferred_issue_execution`). When the lock releases, `releaseIssueExecutionAndPromote()` finds the oldest deferred wakeup and promotes it to queued.

**Relevance to Praetor:** Praetor currently executes tasks sequentially, so task locking is unnecessary. However, the plan schema already supports `depends_on` DAGs, meaning independent tasks could theoretically run in parallel. If concurrent execution is added, two executors could claim the same task without a locking mechanism. Git worktree isolation prevents filesystem conflicts but not logical double-execution.

**Adaptation:** File-based lock per task: `locks/<plan>/<task-id>.lock` containing `{ "run_id": "...", "claimed_at": "...", "pid": N }`. Check-and-create atomically using `os.OpenFile` with `O_CREATE|O_EXCL`. Release on task completion. Stale lock detection via PID liveness check. No deferred promotion needed initially -- if a task is locked, the pipeline simply skips it and moves to the next runnable task. This is simpler than Paperclip's approach but sufficient for Praetor's sequential-first model.

**Applicability:** Medium. Not needed today but foundational for future concurrent execution. Low effort to implement defensively now.

---

## 8. Skill Injection via Temporary Directory Symlinks

**Paperclip approach:** The Claude adapter creates a tmpdir (`/tmp/paperclip-skills-XXXX`), symlinks skill directories into `.claude/skills/`, passes `--add-dir` to the CLI, and cleans up in a `finally` block. This injects skills without polluting the working directory or git state.

**Relevance to Praetor:** Praetor's shared agent commands (`internal/commands`) generate persistent symlinks in `.agents/commands/` with links to `.claude/`, `.cursor/`, `.codex/`. These are project-permanent. For per-task or per-run dynamic instructions (e.g., task-specific tool constraints, custom review criteria), the tmpdir pattern would enable transient injection without workspace pollution.

**Adaptation:** In the CLI adapter's `Execute()` method, create a tmpdir, symlink any task-specific instruction files (from `.praetor/skills/` or generated at runtime), pass as `--add-dir` to the agent CLI, and defer cleanup. Use this for dynamic per-task context that goes beyond what prompt templates provide -- e.g., project-specific coding standards, architecture decision records, or API specifications that an executor should reference. Keep existing `.agents/commands/` for permanent shared commands.

**Applicability:** Medium. Useful for rich per-task context injection. Not urgent -- Praetor's prompt template system handles most cases today.

---

## 9. Wakeup Coalescing and Deduplication

**Paperclip approach:** When multiple triggers arrive for the same agent-task pair, `enqueueWakeup()` merges context into the active run instead of spawning redundant executions. Six distinct outcomes handle every combination of issue-scoped vs. non-issue-scoped, same-agent vs. different-agent, and active vs. queued runs.

**Relevance to Praetor:** Praetor's plan-driven model doesn't have wakeup triggers, so the full coalescing logic is irrelevant. However, the core principle -- deduplicating redundant work on the same task -- applies to retry scenarios. When a task fails and is retried, the new attempt should incorporate context from the failed attempt rather than starting blind. Praetor already does this via retry feedback in prompts, but the concept of "merging context into an active execution" could apply if Praetor evolves toward event-driven or watch-mode operation.

**Adaptation:** No immediate action needed. If a `plan watch` or continuous-mode feature is added, implement a simple deduplication layer: "if same task is already executing, queue the trigger and merge context when the current execution completes." Start with 2 outcomes (execute or queue), not Paperclip's 6.

**Applicability:** Low. Only relevant if Praetor adds event-driven execution. Worth noting as a design principle: "deduplicate, don't duplicate."

---

## 10. Centralized Wakeup Coordination via Single Entrypoint

**Paperclip approach:** All wakeup sources (timer, assignment, on-demand, automation) flow through a single function: `enqueueWakeup()`. This function handles policy checks, agent status validation, source-specific behavior, coalescing, and run creation. No source invokes adapters directly.

**Relevance to Praetor:** Praetor's pipeline FSM already serves as a centralized execution coordinator -- `SelectRunnableTask()` -> `Execute()` -> `Review()` -> `ApplyOutcome()`. However, entry points exist in both `plan run` (FSM loop) and `exec` (direct dispatch) with different code paths. As features are added (MCP-triggered execution, watch mode, external triggers), having a single task-dispatch entrypoint prevents inconsistency.

**Adaptation:** Ensure all execution paths (CLI `plan run`, CLI `exec`, MCP tool invocation, future triggers) converge to a single `DispatchTask()` function that validates preconditions, checks locks, applies agent selection, and invokes the adapter. The FSM's `executeTask` method is already close to this -- formalize it as the canonical entrypoint. No new code needed -- just a design principle to enforce as new features are added.

**Applicability:** Medium. Architectural discipline. Zero implementation cost -- it is about maintaining the pattern that already exists.

---

## 11. Environment Variables as Structured Context Channel

**Paperclip approach:** Each heartbeat run injects 14+ `PAPERCLIP_*` environment variables providing the agent with identity (`AGENT_ID`), context (`TASK_ID`, `WAKE_REASON`, `WAKE_COMMENT_ID`), and credentials (`API_KEY`). This creates a standardized context channel that works across all adapters.

**Relevance to Praetor:** Praetor passes context exclusively via rendered prompt templates and CLI arguments. Environment variables are not used for agent context. For adapters that don't support system prompts well (REST adapters, generic process adapters), environment variables could provide a secondary context channel.

**Adaptation:** Define a set of `PRAETOR_*` environment variables injected into agent subprocesses: `PRAETOR_PLAN`, `PRAETOR_TASK_ID`, `PRAETOR_RUN_ID`, `PRAETOR_PHASE` (execute/review), `PRAETOR_ATTEMPT` (retry count), `PRAETOR_PROJECT_ROOT`. Set these in the `CommandRunner` before spawning agent processes. Agents that are Praetor-aware can read these for richer integration (e.g., an agent could call back via MCP using the run ID). Non-Praetor-aware agents simply ignore them.

**Applicability:** Medium. Low cost, enables richer agent integration. Particularly useful for MCP callback scenarios.

---

## 12. Atomic Operations for Critical State Transitions

**Paperclip approach:** Critical operations use database-level atomicity: issue checkout uses a compound `UPDATE ... WHERE` clause, budget increments use `SET spent = spent + cost`, run claiming uses `UPDATE WHERE status='queued'` to prevent double-claim. All critical operations are single-statement atomic.

**Relevance to Praetor:** Praetor uses file-based state with checkpoint recovery. State transitions (task status changes, snapshot writes) use `os.WriteFile` which is not atomic on all filesystems -- a crash mid-write could corrupt state. The checkpoint ledger provides recovery, but prevention is better than cure.

**Adaptation:** Use atomic file writes for all state mutations: write to a temporary file in the same directory, then `os.Rename()` (which is atomic on POSIX filesystems). Praetor may already do this in some paths -- ensure it is consistent across all state writes (`state/`, `checkpoints/`, `runtime/`). For lock files, use `O_CREATE|O_EXCL` for atomic creation. No new dependencies -- standard Go OS primitives.

**Applicability:** High. Prevents a class of corruption bugs. Minimal implementation effort.

---

## 13. Heartbeat Procedure as Codified Agent Behavior

**Paperclip approach:** The `skills/paperclip/SKILL.md` file codifies the exact procedure every agent follows during a heartbeat: identity check, approval followup, get assignments, pick work, checkout, execute, update, exit. This ensures all agents behave consistently regardless of their underlying model or adapter.

**Relevance to Praetor:** Praetor's executor and reviewer system prompts (`executor.system.tmpl`, `reviewer.system.tmpl`) serve a similar purpose but focus on output format rather than behavioral procedure. The executor is told what format to produce but not given a step-by-step procedure for how to approach the task (e.g., "read existing code first, understand the architecture, then implement, then verify").

**Adaptation:** Enrich the executor system prompt template with a procedural section: "1. Read and understand the relevant code. 2. Plan your approach. 3. Implement the changes. 4. Run tests to verify. 5. Format your output as specified." This is a prompt engineering improvement, not a code change. Could also be implemented as a `.praetor/procedures/` directory with task-type-specific procedures injected via the prompt template.

**Applicability:** Medium. Improves agent behavior consistency. Zero code cost -- prompt template change only.

---

## 14. Cost Event Enrichment with Provider and Model Breakdown

**Paperclip approach:** Each `cost_event` records `provider`, `model`, `inputTokens`, `outputTokens`, `costCents`, `billingCode`, and links to `agentId`, `issueId`, `projectId`, `goalId`. This enables multi-dimensional cost analytics: cost by provider, by model, by project, by agent.

**Relevance to Praetor:** Praetor's `AgentResponse` already contains `Usage` data (input/output tokens) and the `performance.jsonl` records timing. But cost data is not enriched with provider/model metadata in a structured way. The `plan diagnose --query costs` command works but lacks the dimensionality for meaningful analytics.

**Adaptation:** Enrich the cost metrics written to `costs/<plan>/<run-id>.jsonl` with `provider`, `model`, `phase` (execute/review), and `task_id` fields. These are already available in the pipeline context -- just need to be written alongside the token counts. The `plan diagnose --query costs` and `praetor eval` commands can then aggregate by any dimension. No new storage mechanism -- enrichment of existing data.

**Applicability:** High. Low effort, significantly improves cost analytics. All data is already available in the pipeline.

---

## Summary Table

| # | Learning | Applicability | Effort | Priority |
|---|----------|--------------|--------|----------|
| 1 | Session persistence for task-scoped state | High | Medium | High |
| 2 | Adapter session codec pattern | High | Low | High |
| 3 | Graceful session retry on resume failure | High | Low | High |
| 4 | Cumulative agent statistics across runs | High | Low | High |
| 5 | Monetary budget tracking with auto-stop | High | Low | High |
| 6 | Activity logging with actor types | Medium | Low | Medium |
| 7 | Task execution lock with deferred promotion | Medium | Low | Medium |
| 8 | Skill injection via tmpdir symlinks | Medium | Medium | Low |
| 9 | Wakeup coalescing and deduplication | Low | Medium | Low |
| 10 | Centralized wakeup coordination | Medium | Zero | Medium |
| 11 | Environment variables as context channel | Medium | Low | Medium |
| 12 | Atomic file writes for state transitions | High | Low | High |
| 13 | Codified agent behavioral procedures | Medium | Zero | Medium |
| 14 | Cost event enrichment with provider/model | High | Low | High |

### Praetor Constraints Compliance

All 14 learnings respect Praetor's constraints:

- **No PostgreSQL:** All state is file-based (JSON, JSONL). Learnings 1-5, 7, 12 use filesystem primitives.
- **No web framework:** No HTTP server, WebSocket, or React UI. Learnings are CLI-compatible.
- **No new dependencies:** All adaptations use Go stdlib (`os`, `encoding/json`, `crypto/sha256`). No external packages.
- **Single binary:** No additional services, daemons, or background processes.
- **Cobra-only CLI:** All user-facing features integrate into existing Cobra command tree.

### Implementation Order Recommendation

Phase 1 (foundation): #2 (session codec) -> #1 (session persistence) -> #3 (graceful retry) -> #12 (atomic writes)
Phase 2 (cost control): #5 (monetary budget) -> #14 (cost enrichment) -> #4 (cumulative stats)
Phase 3 (observability): #6 (actor-typed logging) -> #11 (env var context) -> #13 (behavioral procedures)
Phase 4 (concurrency prep): #7 (task locks) -> #10 (centralized dispatch) -> #9 (deduplication)
Phase 5 (advanced): #8 (skill injection)
