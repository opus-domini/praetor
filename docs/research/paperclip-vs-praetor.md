# Comparative Analysis: Paperclip vs Praetor

Research output for TASK-005. Synthesizes findings from TASK-001 through TASK-004.

---

## 1. Philosophy and Vision

| Dimension | Paperclip | Praetor |
|-----------|-----------|---------|
| Core metaphor | "The company" -- agents are employees in a corporate structure | "The pipeline" -- agents are executors in a plan-driven workflow |
| Unit of work | Issue on a Kanban board | Task in a sequential plan |
| Human role | Board of directors governing an AI company | Developer/operator running orchestrated plans |
| Agent autonomy | High -- agents self-organize, delegate, create sub-issues | Low -- agents execute prescribed tasks, reviewers gate output |
| Scope | Multi-company, multi-agent organizational layer | Single-project, single-plan task execution |
| Deployment model | Web server + UI + CLI (always-on) | CLI tool (run-and-done) |
| Persistence | PostgreSQL database (durable, queryable) | Filesystem (JSON files, JSONL logs) |
| Design stance | Feature-rich platform (35 tables, 11 roles, 7 adapters) | Minimal, composable tool (one binary, 9 providers) |

### Philosophical tension

Paperclip assumes agents are autonomous enough to decompose work, hire subordinates, and self-organize within guardrails. This requires sophisticated governance (budgets, approvals, org hierarchy) because the blast radius of uncontrolled autonomy is large.

Praetor assumes agents need tight guidance -- a human (or planner agent) creates the plan, and agents execute within strict boundaries. Governance is lightweight because the blast radius is small: one task at a time, gated by automated review.

---

## 2. Technology Stack

| Dimension | Paperclip | Praetor |
|-----------|-----------|---------|
| Language | TypeScript (Node.js >= 20) | Go |
| Build system | pnpm workspaces (monorepo) | Single Go module |
| Binary distribution | npm (`npx paperclipai`) | `go install` (single static binary) |
| Database | PostgreSQL 17 (prod) / PGlite (dev) | None (filesystem) |
| ORM | Drizzle ORM (30+ tables, 24 migrations) | N/A |
| HTTP framework | Express.js 5 | None (CLI-only) |
| Frontend | React 19 + Vite + Tailwind + Radix | None |
| Auth | Better Auth (sessions, OAuth) | None (local trust) |
| WebSockets | ws library (live events) | None |
| Validation | Zod schemas | Go struct tags + `DisallowUnknownFields()` |
| Test framework | Vitest | Go stdlib `testing` |
| Logging | pino (structured JSON) | slog / JSONL event files |
| Config format | Environment variables + runtime config JSON | TOML flat file with project sections |
| Process communication | JSON-RPC (MCP) + REST + WebSocket | JSON-RPC (MCP) + CLI stdio |
| Documentation | Mintlify (hosted) | Markdown in `docs/` |

### Complexity profile

Paperclip's stack is typical of a web application: database, API server, SPA frontend, WebSocket layer, auth system. This adds operational overhead (migrations, connection pools, deployment, auth flows) but enables rich UI, multi-user access, and durable state.

Praetor's stack is deliberately minimal: one Go binary, no database, no server. State lives in files. This means zero operational overhead but limits multi-user access and real-time collaboration.

---

## 3. Agent Model

| Dimension | Paperclip | Praetor |
|-----------|-----------|---------|
| Agent identity | Named entities with roles, budgets, permissions, org position | Anonymous provider adapters (identified by provider name) |
| Agent lifecycle | 7 states (idle, active, running, paused, error, pending_approval, terminated) | Stateless -- probed at startup, invoked per task |
| Agent creation | Approval-gated (hire_agent flow) | Implicit via config/CLI flags |
| Agent roles | 11 predefined (CEO, CTO, engineer, QA, etc.) | 3 functional roles (executor, reviewer, planner) |
| Agent hierarchy | Tree (`reportsTo` self-referential FK with cycle prevention) | Flat (executor + reviewer, no hierarchy) |
| Multi-agent coordination | Issue-based delegation, @mentions, sub-issues | Plan-level sequencing, per-task agent overrides |
| Agent configuration | Per-agent adapter config, runtime config, heartbeat policy | Global config + per-task agent overrides in plan |
| Session persistence | DB-backed per `(agent, adapter, taskKey)` | None (each invocation is fresh) |
| Provider support | 5 adapters (Claude, Codex, Cursor, OpenClaw, OpenCode) + 2 generic (process, HTTP) | 9 providers (Claude, Codex, Copilot, Gemini, Kimi, OpenCode, OpenRouter, Ollama, LM Studio) |
| Adapter interface | 3-module pattern (server/UI/CLI) with session codec | Single `Agent` interface with `Plan()/Execute()/Review()` |
| Fallback handling | No automatic fallback | Error-classified fallback runtime (transient, auth, rate_limit) |
| Health checking | `testEnvironment()` per adapter | `agent.Prober` with version parsing + health endpoints |

### Key differences

Paperclip treats agents as persistent entities with identity, state, and organizational position. An agent accumulates history (total tokens, costs, session state) and exists across many heartbeats. This enables rich organizational dynamics but adds complexity.

Praetor treats agents as interchangeable execution backends. The same task could run on Claude, Codex, or Gemini -- the pipeline doesn't care. This enables wide provider support and graceful fallback but sacrifices agent specialization.

---

## 4. Task and Plan Model

| Dimension | Paperclip | Praetor |
|-----------|-----------|---------|
| Work decomposition | Goals -> Projects -> Issues (3-level hierarchy) | Plans -> Tasks (2-level, flat within plan) |
| Task schema | 15+ fields (status, priority, assignee, parent, project, goal, labels, attachments, billing code) | 6 fields (id, title, description, acceptance, depends_on, constraints) |
| Dependencies | Implicit via sub-issues (`parentId`) | Explicit DAG via `depends_on` array |
| Task states | 7 (backlog, todo, in_progress, in_review, done, blocked, cancelled) | 4 (pending, executing, reviewing, done, failed) |
| Task assignment | Agent-based (assignee field, atomic checkout) | Automatic (pipeline selects next runnable task) |
| Concurrency | Multiple agents work on different issues simultaneously | Sequential task execution within a plan |
| Work isolation | Issue execution lock (DB row lock) | Git worktree per task |
| Acceptance criteria | Free-text description | Structured array + quality gates + reviewer verdict |
| Quality gates | None (delegated to agent judgment) | Host-executed commands (tests, lint, standards) |
| Review model | Comments and @mentions (peer review via agents) | Dedicated reviewer agent with PASS/FAIL decision |
| Retry mechanism | Manual (reopen issue, reassign) | Automatic (up to `max_retries`, with stall detection) |
| Stall detection | None | Fingerprint-based sliding window with 3-level escalation |
| Budget management | Per-agent and per-company monthly ceilings (auto-pause) | Context budget per phase (char-based truncation) |
| Cost tracking | Dedicated `cost_events` table with provider/model/token breakdown | JSONL metrics files per run |
| Plan creation | CEO agent decomposes goals into issues (organic) | Planner agent or manual via `plan create` (structured) |
| Cognitive metadata | None | `cognitive` block: assumptions, open_questions, failure_modes, decisions |

### Key differences

Paperclip's task model is optimized for ongoing, open-ended work. Issues live on a board, agents pick them up, delegate sub-tasks, and communicate through comments. There is no predefined execution order -- work flows organically.

Praetor's task model is optimized for deterministic execution. Tasks have explicit dependencies, acceptance criteria, quality gates, and a strict execute-review loop. The plan defines everything upfront, and the pipeline drives execution to completion.

---

## 5. Governance and Control

| Dimension | Paperclip | Praetor |
|-----------|-----------|---------|
| Governance model | Human board governs AI agents (corporate metaphor) | Automated pipeline with optional human review |
| Approval types | `hire_agent`, `approve_ceo_strategy` (explicit board gates) | None (automated review replaces approvals) |
| Budget enforcement | 2-tier monetary (company + agent), auto-pause at 100% | Context-char budgets per phase, no monetary tracking |
| Spending visibility | Dashboard with per-agent, per-project, per-model breakdowns | `plan diagnose --query costs` CLI output |
| Audit trail | `activity_log` table (every mutation, actor-typed) | JSONL event files + checkpoint ledger |
| Permission model | RBAC with `principal_permission_grants` (fine-grained) | None (single-user, local trust) |
| Agent constraints | Organizational (role, hierarchy, budget) | Per-task tool constraints (`allowed_tools`, `denied_tools`) |
| Secrets management | `company_secrets` table (encrypted, versioned) | Environment variables (delegated to OS) |
| Multi-tenancy | Multiple companies per instance | Single project at a time |
| Recovery model | Stale tasks surfaced to board, no auto-recovery | Snapshot-based state recovery, transient states reset to pending |
| Error escalation | Agent marks issue as `blocked`, board decides | Stall detection escalates (fallback -> budget reduce -> force fail) |

### Key differences

Paperclip's governance assumes autonomous agents need guardrails: budgets prevent runaway spending, approvals gate high-impact actions, the org hierarchy structures delegation. The board is the ultimate authority.

Praetor's governance is automated: quality gates replace human approval, stall detection replaces manual intervention, fallback agents replace manual error recovery. The human only intervenes when the pipeline fails entirely.

---

## 6. Communication and Integration

| Dimension | Paperclip | Praetor |
|-----------|-----------|---------|
| Agent-to-agent communication | Issues (comments, @mentions, sub-issues) | None (agents don't communicate directly) |
| Agent-to-system communication | REST API (bidirectional) + env vars | System prompt + CLI args (unidirectional) + MCP |
| Agent context injection | PAPERCLIP_* env vars (14+ variables) | Rendered prompt templates (executor.task.tmpl, reviewer.task.tmpl) |
| Skill system | SKILL.md files with YAML frontmatter, injected via tmpdir symlinks | Shared agent commands (`.agents/commands/`) with symlinks |
| MCP support | Not implemented | Built-in MCP server (JSON-RPC 2.0 over stdio) |
| Real-time updates | WebSocket live events | None (CLI polling via `plan diagnose`) |
| External integrations | REST API + webhooks (HTTP adapter) | MCP tools/resources for AI agent access |
| UI | Full React SPA (dashboard, Kanban, org chart, cost analytics) | CLI-only (terminal tables, formatted output) |

### Key differences

Paperclip is deeply interactive: agents communicate with each other and the system through a rich API. The UI provides real-time visibility. This enables emergent coordination but requires a running server.

Praetor is batch-oriented: the pipeline injects context via prompts, agents produce output, the pipeline parses and acts. Communication is unidirectional per phase. MCP provides integration points but not real-time coordination.

---

## 7. What Each Project Does Better

### Paperclip does better (12 items)

1. **Agent identity and state** -- Named agents with persistent identity, session history, and cumulative statistics enable rich organizational dynamics and debugging.

2. **Organizational modeling** -- Tree hierarchy with `reportsTo`, 11 roles, and chain-of-command queries models real organizations.

3. **Work decomposition** -- Goals -> Projects -> Issues hierarchy with sub-issue delegation creates natural work breakdown structures.

4. **Inter-agent communication** -- Issue comments with @mentions and wakeup triggers enable asynchronous agent collaboration.

5. **Budget enforcement** -- Monetary budget tracking with auto-pause prevents runaway spending. Per-agent and per-company ceilings provide layered control.

6. **Approval workflows** -- Board approval gates for high-impact actions (hiring, strategy) provide human oversight at critical junctures.

7. **Session persistence** -- Task-scoped session state enables agents to resume conversations across heartbeats without context loss.

8. **Wakeup coalescing** -- Deduplicating and deferring redundant wakeups prevents wasted computation. Six-outcome coalescing logic is sophisticated.

9. **Real-time visibility** -- WebSocket live events + React dashboard provide immediate operational awareness.

10. **Audit trail** -- Actor-typed activity log with entity indexing enables forensic analysis of every mutation.

11. **Concurrent multi-issue execution** -- Multiple agents work on different issues simultaneously with issue-level execution locks.

12. **Skill injection protocol** -- Tmpdir symlink pattern cleanly injects skills without polluting working directories.

### Praetor does better (13 items)

1. **Provider breadth** -- 9 providers (vs 5+2 generic) covering both CLI and REST agents, including local models (Ollama, LM Studio) and routing (OpenRouter).

2. **Automated quality gates** -- Host-executed test/lint/standards commands provide objective pass/fail signals without relying on agent judgment.

3. **Stall detection** -- Fingerprint-based sliding window with 3-level escalation (fallback -> budget reduce -> force fail) automatically breaks execution loops.

4. **Fallback runtime** -- Error-classified automatic fallback (transient -> fallback agent, auth -> fallback agent) provides resilience without human intervention.

5. **Deterministic execution** -- Explicit DAG dependencies, plan-level sequencing, and snapshot recovery ensure repeatable, predictable task execution.

6. **Zero operational overhead** -- Single Go binary, no database, no server, no auth. Install and run immediately.

7. **Cognitive metadata** -- Plans capture assumptions, open questions, failure modes, and decisions -- making planning rationale explicit and auditable.

8. **Per-task tool constraints** -- `allowed_tools` and `denied_tools` enforce least-privilege execution at the task level.

9. **MCP integration** -- Built-in MCP server exposes plans, state, and diagnostics to any MCP-aware AI agent.

10. **Context budget management** -- Character-level prompt truncation with phase-specific strategies prevents context overflow.

11. **Evaluation framework** -- `plan eval` and `praetor eval` provide structured quality assessment with verdicts (`pass|warn|fail`) at plan and project levels.

12. **Configuration layering** -- CLI flags > project config > global config > plan settings > defaults provides predictable, debuggable configuration.

13. **Git worktree isolation** -- Each task executes in an isolated worktree, preventing agent interference at the filesystem level.

---

## 8. Concepts to Absorb from Paperclip

### 8.1 Session persistence for task-scoped agent state

**What:** Persist agent session state keyed on `(agent, adapter, taskKey)` so agents can resume conversations across invocations.

**Why:** Praetor currently treats every invocation as stateless. For multi-heartbeat tasks or retry scenarios, agents lose context between runs. Session persistence would enable Claude's `--resume` feature and reduce redundant context injection.

**Adaptation:** Implement as filesystem-backed session store (JSON files) rather than database tables, consistent with Praetor's storage model. Add session codec per adapter.

### 8.2 Wakeup coalescing / deduplication

**What:** When multiple triggers arrive for the same task-agent pair, merge context into the active run instead of spawning redundant executions.

**Why:** As Praetor evolves toward event-driven or continuous execution (beyond single plan runs), deduplication will prevent wasted computation. Even now, retry loops could benefit from context merging.

**Adaptation:** Implement at the pipeline level as a run deduplication layer. Simpler than Paperclip's 6-outcome model -- start with "if same task already running, merge context."

### 8.3 Agent identity with cumulative state

**What:** Track per-agent cumulative statistics (total tokens, cost, runs) across plan executions.

**Why:** Enables cost attribution, performance comparison across providers, and informed agent selection. Currently Praetor's cost data is scattered across run-specific files.

**Adaptation:** Add a per-agent state file under the project directory that accumulates across runs. No database needed.

### 8.4 Skill injection via tmpdir symlinks

**What:** Inject skill/instruction files into agent context using temporary directories with symlinks, cleaned up after execution.

**Why:** Praetor's shared commands (`/.agents/commands/`) serve a similar purpose but are project-permanent rather than per-invocation. The tmpdir pattern would enable dynamic, run-specific skill injection without polluting the workspace.

**Adaptation:** Consider for dynamic per-task instructions beyond what prompt templates provide. Low priority -- Praetor's prompt template system already handles most cases.

### 8.5 Issue execution lock pattern

**What:** Database-level (or file-level) lock ensuring only one agent works on a given task at a time, with deferred promotion when the lock releases.

**Why:** As Praetor evolves toward concurrent task execution (parallel independent tasks in a plan), it needs a mechanism to prevent two agents from claiming the same task. Git worktree isolation prevents filesystem conflicts but not logical double-execution.

**Adaptation:** File-based lock per task ID, checked before execution begins. Simple compared to Paperclip's DB row lock.

### 8.6 Activity logging with actor types

**What:** Record every significant mutation with actor type (agent/user/system), entity type, and structured details.

**Why:** Praetor's JSONL event files capture execution events but not mutation provenance. Actor-typed audit trails would improve debugging and accountability.

**Adaptation:** Extend the existing event schema to include `actor_type` and `entity_type` fields. No new storage needed.

### 8.7 Monetary budget tracking

**What:** Track actual LLM spending in cents with per-provider, per-model breakdowns. Auto-pause agents that exceed budgets.

**Why:** Praetor tracks context-char budgets for prompt sizing but not actual monetary costs. As plans grow larger, cost visibility becomes critical.

**Adaptation:** Enrich the existing cost metrics files with provider-reported cost data. Add optional budget ceilings in plan settings or config. Simpler than Paperclip's 2-tier model -- one ceiling per plan run.

---

## 9. Concepts to Discard from Paperclip

### 9.1 Corporate hierarchy metaphor (CEO, CTO, etc.)

**Why discard:** The 11-role hierarchy assumes a specific organizational model (AI company) that doesn't map to Praetor's purpose. Praetor orchestrates tasks, not organizations. Adding roles like "CEO" and "CTO" would introduce complexity without benefit for plan-driven execution.

**What to keep instead:** Praetor's functional roles (executor, reviewer, planner) are sufficient and provider-agnostic.

### 9.2 Human board approval gates

**Why discard:** Paperclip's approval system assumes an always-on server with human board operators monitoring agent actions. Praetor is a CLI tool designed for automated execution. Adding board approval gates would break the batch-execution model and require a running server.

**What to keep instead:** Praetor's automated quality gates (tests, lint, standards) and reviewer agent provide equivalent safety without human intervention.

### 9.3 PostgreSQL database and Drizzle ORM

**Why discard:** Praetor's filesystem-based state (JSON, JSONL, checkpoints) is deliberately zero-dependency. Adding a database would contradict the "single binary, no infrastructure" design principle. The operational overhead of migrations, connection management, and deployment is not justified.

**What to keep instead:** Continue using structured JSON files with integrity checks (checksums, snapshot recovery).

### 9.4 React SPA and WebSocket UI

**Why discard:** Building a web UI would be a massive scope expansion with ongoing maintenance burden. Praetor's CLI-first philosophy serves its audience (developers who live in terminals) better than a dashboard.

**What to keep instead:** MCP integration provides programmatic access to all state. Future TUI could be built with terminal libraries if needed.

### 9.5 RBAC and multi-user authentication

**Why discard:** Praetor runs as a local CLI tool under the user's own permissions. Multi-user auth adds complexity for a use case that doesn't exist (single developer running plans locally).

**What to keep instead:** OS-level permissions. If multi-user is needed in the future, it belongs in a separate deployment layer, not core Praetor.

### 9.6 Company-level multi-tenancy

**Why discard:** Running multiple "companies" in a single Praetor instance has no parallel. Praetor's project isolation (one project directory per project) is simpler and sufficient.

**What to keep instead:** Project-level configuration sections in `config.toml`.

### 9.7 Agent hiring and onboarding workflow

**Why discard:** The approval-gated hire_agent flow assumes agents are persistent entities that join an organization. Praetor's agents are transient -- probed at startup, used during execution, discarded after. No hiring needed.

**What to keep instead:** `praetor doctor` for agent health checking and the config-based agent selection.

### 9.8 Heartbeat-based scheduling

**Why discard:** Paperclip's heartbeat protocol (timer-based wakes, interval policies, max concurrent runs) is designed for always-on agent orchestration. Praetor's execution is plan-driven -- run starts, tasks execute, run ends. There is no idle period to wake agents from.

**What to keep instead:** Praetor's plan-driven execution loop. If continuous operation is needed, it should be driven by an external scheduler (cron, systemd timer) invoking `praetor plan run`.

### 9.9 Issue-based inter-agent communication

**Why discard:** Paperclip's model of agents communicating through issue comments and @mentions requires persistent state, a running server, and the organizational hierarchy. Praetor's agents don't need to communicate with each other -- the pipeline mediates all information flow.

**What to keep instead:** Praetor's prompt-mediated context passing (executor output -> reviewer input).

---

## 10. Strategic Summary

### Praetor's identity

Praetor is a **task execution engine**: take a plan, execute it deterministically, verify quality, report results. Its strength is reliability, predictability, and zero operational overhead.

### Paperclip's identity

Paperclip is an **organizational simulation**: model a company, let agents self-organize within governance guardrails, provide visibility through dashboards. Its strength is autonomous multi-agent coordination at scale.

### Convergence path

The most valuable Paperclip concepts for Praetor are those that improve **execution quality** without sacrificing **operational simplicity**:

| Priority | Concept | Effort | Impact |
|----------|---------|--------|--------|
| High | Monetary budget tracking | Low | Cost visibility for plan runs |
| High | Agent cumulative state | Low | Cross-run performance analytics |
| High | Session persistence | Medium | Better retry/resume behavior |
| Medium | Activity logging with actor types | Low | Richer audit trail |
| Medium | Task execution locks | Low | Foundation for concurrent execution |
| Low | Wakeup coalescing | Medium | Only relevant if event-driven model added |
| Low | Skill injection via tmpdir | Low | Marginal improvement over prompt templates |

The concepts to discard are those that assume an **always-on organizational layer** -- the corporate hierarchy, board approvals, database-backed state, web UI, and inter-agent communication. These are core to Paperclip's identity but antithetical to Praetor's design philosophy.
