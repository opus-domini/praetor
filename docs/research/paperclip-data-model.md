# Paperclip Data Model, Communication Patterns, and Skill Protocol

Research output for TASK-004. Source: https://github.com/paperclipai/paperclip

---

## 1. Data Model Layers (35 Tables)

The PostgreSQL schema is organized into five conceptual layers. Every table except auth tables carries a `companyId` foreign key, enforcing company-level isolation.

### 1.1 Layer 1: Organizational (8 tables)

Tables that define the company, its members, and their permissions.

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `companies` | Top-level company entity | `name`, `status` (active/paused/archived), `issuePrefix`, `issueCounter`, `budgetMonthlyCents`, `spentMonthlyCents`, `requireBoardApprovalForNewAgents` |
| `agents` | AI agent definitions | `name`, `role` (11 roles), `status` (7 states), `reportsTo` (self-ref FK), `adapterType`, `adapterConfig` (jsonb), `runtimeConfig` (jsonb), `budgetMonthlyCents`, `spentMonthlyCents`, `permissions` (jsonb) |
| `company_memberships` | Principal membership in company | `principalType` (agent/user), `principalId`, `status` (active/pending/suspended), `membershipRole` |
| `instance_user_roles` | Instance-level admin roles | `userId`, `role` (instance_admin). Short-circuits all company authz. |
| `principal_permission_grants` | Fine-grained RBAC | `principalType`, `principalId`, `permissionKey` (agents:create, tasks:assign, etc.), `scope` (jsonb) |
| `invites` | Invite links for joining | `tokenHash` (unique), `inviteType` (company_join), `allowedJoinTypes`, `defaultsPayload`, `expiresAt` |
| `join_requests` | Pending join applications | `requestType`, `status` (pending_approval/approved/rejected), `agentName`, `adapterType`, `claimSecretHash` |
| `labels` | Company-scoped issue labels | `name` (unique per company), `color` |

### 1.2 Layer 2: Work Hierarchy (8 tables)

Tables that model the goals-to-issues work hierarchy.

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `goals` | Strategic objectives | `title`, `level` (task default), `status` (planned/active/achieved/cancelled), `parentId` (self-ref FK), `ownerAgentId` |
| `projects` | Scoped work containers | `name`, `status` (backlog/planned/in_progress/completed/cancelled), `leadAgentId`, `goalId`, `targetDate`, `archivedAt` |
| `project_goals` | M:N join: projects-goals | `projectId`, `goalId` (composite PK) |
| `project_workspaces` | Filesystem/repo per project | `name`, `cwd`, `repoUrl`, `repoRef`, `isPrimary`. Resolved during heartbeat for agent working directory. |
| `issues` | Atomic work items (the core entity) | See section 2 below for full breakdown. |
| `issue_comments` | Comment threads on issues | `issueId`, `authorAgentId`/`authorUserId`, `body` |
| `issue_labels` | M:N join: issues-labels | `issueId`, `labelId` (composite PK) |
| `issue_attachments` | File attachments on issues | `issueId`, `assetId`, `issueCommentId` (optional, links to comment) |

### 1.3 Layer 3: Governance (4 tables)

Tables for board oversight and approval workflows.

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `approvals` | Board approval gates | `type` (hire_agent/approve_ceo_strategy), `status` (pending/revision_requested/approved/rejected/cancelled), `requestedByAgentId`, `payload` (jsonb), `decisionNote`, `decidedByUserId` |
| `approval_comments` | Discussion thread per approval | `approvalId`, `authorAgentId`/`authorUserId`, `body` |
| `issue_approvals` | M:N join: issues-approvals | `issueId`, `approvalId` (composite PK). Traceability link. |
| `cost_events` | LLM cost tracking records | `agentId`, `issueId`, `projectId`, `goalId`, `billingCode`, `provider`, `model`, `inputTokens`, `outputTokens`, `costCents`, `occurredAt` |

### 1.4 Layer 4: Execution (7 tables)

Tables for agent scheduling, running, and session management.

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `agent_wakeup_requests` | Wakeup queue entries | `source` (timer/assignment/on_demand/automation), `triggerDetail`, `reason`, `status` (queued/deferred_issue_execution/claimed/coalesced/skipped/completed/failed/cancelled), `coalescedCount`, `idempotencyKey` |
| `heartbeat_runs` | Individual execution records | `agentId`, `invocationSource`, `status` (queued/running/succeeded/failed/cancelled/timed_out), `exitCode`, `usageJson`, `resultJson`, `sessionIdBefore`/`After`, `contextSnapshot` (jsonb), `logStore`/`logRef`/`logBytes`/`logSha256` |
| `heartbeat_run_events` | Streaming log entries per run | `runId`, `seq`, `eventType`, `stream`, `level`, `message`, `payload` (jsonb) |
| `agent_runtime_state` | Per-agent cumulative state | `agentId` (PK), `adapterType`, `sessionId`, `stateJson`, `lastRunId`, `lastRunStatus`, `totalInputTokens`/`OutputTokens`/`CachedInputTokens`/`CostCents`, `lastError` |
| `agent_task_sessions` | Per-task session persistence | `agentId`, `adapterType`, `taskKey` (unique triple), `sessionParamsJson`, `sessionDisplayId`, `lastRunId` |
| `agent_config_revisions` | Config change audit trail | `agentId`, `source` (patch), `changedKeys`, `beforeConfig`, `afterConfig`, `createdByAgentId`/`UserId` |
| `agent_api_keys` | Agent authentication keys | `agentId`, `name`, `keyHash`, `lastUsedAt`, `revokedAt` |

### 1.5 Layer 5: Security & Infrastructure (4 tables)

Tables for secrets, assets, and auth sessions.

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `company_secrets` | Named secrets per company | `name` (unique per company), `provider` (local_encrypted), `latestVersion`, `description` |
| `company_secret_versions` | Versioned secret material | `secretId`, `version`, `material` (jsonb, encrypted), `valueSha256`, `revokedAt` |
| `assets` | Uploaded files (local disk or S3) | `provider`, `objectKey`, `contentType`, `byteSize`, `sha256`, `originalFilename` |
| `activity_log` | Full audit trail | `actorType` (agent/user/system), `actorId`, `action` (dot-notation), `entityType`, `entityId`, `details` (jsonb) |

### 1.6 Auth Tables (4 tables, from Better Auth)

`user`, `session`, `account`, `verification` -- standard Better Auth tables for human session management.

---

## 2. Communication Model: Issues as the Sole Channel

Paperclip's core design principle: **agents communicate exclusively through issues**. There is no direct agent-to-agent messaging. All coordination flows through the issue lifecycle.

### 2.1 The Issue Entity (Full Schema)

The `issues` table is the most connected table in the schema:

```
issues
  id                         uuid PK
  companyId                  uuid FK -> companies
  projectId                  uuid FK -> projects (optional)
  goalId                     uuid FK -> goals (optional)
  parentId                   uuid FK -> issues (self-ref, sub-issue chain)
  title                      text
  description                text (nullable)
  status                     text: backlog|todo|in_progress|in_review|done|blocked|cancelled
  priority                   text: critical|high|medium|low
  assigneeAgentId            uuid FK -> agents (nullable)
  assigneeUserId             text (nullable, mutually exclusive with agent)
  checkoutRunId              uuid FK -> heartbeat_runs (execution lock)
  executionRunId             uuid FK -> heartbeat_runs (current executing run)
  executionAgentNameKey      text (name of executing agent)
  executionLockedAt          timestamp
  createdByAgentId           uuid FK -> agents (nullable)
  createdByUserId            text (nullable)
  issueNumber                integer (sequential per company)
  identifier                 text UNIQUE (e.g. "PAP-39")
  requestDepth               integer (delegation chain depth)
  billingCode                text (cost allocation tag)
  assigneeAdapterOverrides   jsonb (per-issue adapter config)
  startedAt / completedAt / cancelledAt / hiddenAt  timestamps
```

### 2.2 Communication Primitives

| Primitive | Mechanism | Effect |
|-----------|-----------|--------|
| **Assign work** | Set `assigneeAgentId` on create/update | Triggers wakeup with `reason: "issue_assigned"` |
| **Delegate** | Create sub-issue with `parentId` + assign to subordinate | New issue in subordinate's inbox, `requestDepth` incremented |
| **Discuss** | POST comment on issue | @mention extraction triggers wakeup for mentioned agents |
| **Acknowledge** | Checkout issue (atomic) | Sets `status = in_progress`, locks to specific run |
| **Complete** | Update status to `done` | Sets `completedAt`, releases lock |
| **Block/Escalate** | Update status to `blocked` + comment | Parent agent sees blocked child in next heartbeat |
| **Return** | Release issue (reset to `todo`, clear assignee) | Issue returns to board/creator's view |

### 2.3 Delegation via Sub-Issues

Delegation follows the org hierarchy:

```
CEO creates issue "Build payment system" (PAP-1)
  └─ Assigns to CTO
       CTO creates sub-issue "Design API" (PAP-2, parentId=PAP-1)
         └─ Assigns to Engineer
              Engineer creates sub-issue "Write tests" (PAP-3, parentId=PAP-2)
                └─ Assigns to QA agent
```

Key mechanics:
- `parentId` creates arbitrary-depth chains (cycle detection, 50-hop limit)
- `requestDepth` tracks delegation depth (0 = top-level)
- `getAncestors()` walks up the parent chain for context
- Sub-issues are independently assignable, checkable, and completable
- No explicit team boundary enforcement within a company -- any agent can be assigned any issue

### 2.4 Comments and @Mentions

Comments are the dialogue mechanism. The system extracts @mentions using regex `\B@([^\s@,!?.]+)` and wakes mentioned agents:

```
Agent A posts on PAP-5: "@Engineer please review the API spec before I proceed"
  -> System extracts "Engineer"
  -> Matches against agent names (case-insensitive)
  -> Fires wakeup: source="automation", reason="issue_comment_mentioned"
  -> Engineer's next heartbeat sees PAP-5 in assignments with new comment
```

Special comment features:
- `reopen: true` -- reopens done/cancelled issues to `todo`
- `interrupt: true` -- board-only, cancels active run on the issue
- Comments carry `authorAgentId` or `authorUserId` for attribution

### 2.5 Atomic Checkout (The Execution Lock)

Checkout prevents concurrent work on the same issue:

```
POST /issues/:id/checkout
{ "agentId": "uuid", "expectedStatuses": ["backlog", "todo"] }
```

Atomicity is enforced by a single UPDATE with multiple WHERE conditions:
1. Issue status is in `expectedStatuses`
2. Agent is unassigned OR already assigned with same run lock
3. No execution lock OR same run already holds it

On success: `status = "in_progress"`, `checkoutRunId` and `executionRunId` set, `startedAt` set.
On conflict: 409 with current state (another agent holds the lock).

**Stale lock recovery**: If the old `checkoutRunId` references a terminal run (succeeded/failed/cancelled/timed_out), the new run can adopt the lock. Logged as `issue.checkout_lock_adopted`.

### 2.6 The Heartbeat Procedure (Agent's Communication Loop)

Every heartbeat, each agent follows a standardized procedure (defined in `skills/paperclip/SKILL.md`):

1. **Identity** -- `GET /api/agents/me` for id, company, role, chain of command, budget
2. **Approval follow-up** -- If `PAPERCLIP_APPROVAL_ID` is set, handle approval resolution
3. **Get assignments** -- `GET /api/companies/{id}/issues?assigneeAgentId={me}&status=todo,in_progress,blocked`
4. **Pick work** -- Prioritize `in_progress` > `todo` > `blocked` (skip if no new context)
5. **Checkout** -- Atomically claim the issue
6. **Execute** -- Do the actual work (code, research, etc.)
7. **Update** -- Report results via status update, comments, or sub-issue creation
8. **Exit** -- Agent process exits, session persisted for next heartbeat

### 2.7 Environment Variables as Context Channel

Each heartbeat run injects context via environment variables:

| Variable | Purpose |
|----------|---------|
| `PAPERCLIP_AGENT_ID` | Agent's UUID |
| `PAPERCLIP_COMPANY_ID` | Company UUID |
| `PAPERCLIP_API_URL` | Server base URL |
| `PAPERCLIP_RUN_ID` | Current heartbeat run UUID |
| `PAPERCLIP_API_KEY` | Short-lived JWT (auto-injected for local adapters) |
| `PAPERCLIP_TASK_ID` | Issue that triggered this wake |
| `PAPERCLIP_WAKE_REASON` | Why this run was triggered |
| `PAPERCLIP_WAKE_COMMENT_ID` | Specific comment that triggered wake |
| `PAPERCLIP_APPROVAL_ID` | Approval being resolved |
| `PAPERCLIP_APPROVAL_STATUS` | Approval decision |
| `PAPERCLIP_LINKED_ISSUE_IDS` | Comma-separated related issues |

---

## 3. Skill Injection Protocol

Skills are packaged procedures that teach agents how to interact with Paperclip and follow company workflows. They are injected into every agent run via the adapter.

### 3.1 Skill Directory Structure

```
skills/
  paperclip/                      # Core Paperclip API coordination
    SKILL.md                      # Heartbeat procedure + API usage
    references/
      api-reference.md            # Full endpoint table with examples
  paperclip-create-agent/         # Agent hiring workflow
    SKILL.md                      # Hire request + approval lifecycle
    references/
      api-reference.md            # Hire-specific endpoints
  create-agent-adapter/           # Adapter authoring guide
    SKILL.md                      # 700+ lines, full adapter creation spec
  para-memory-files/              # File-based memory system
    SKILL.md                      # PARA-method memory layers
    references/
      schemas.md                  # YAML fact schemas, decay rules
```

### 3.2 SKILL.md Format

Each skill file uses YAML frontmatter with two fields:

```yaml
---
name: paperclip
description: >
  Interact with the Paperclip control plane API to manage tasks, coordinate with
  other agents, and follow company governance. Use when you need to check
  assignments, update task status, delegate work, post comments, or call any
  Paperclip API endpoint. Do NOT use for the actual domain work itself (writing
  code, research, etc.) -- only for Paperclip coordination.
---

# Skill Title

<detailed procedures, rules, and examples>
```

- **`name`**: Unique identifier (snake_case)
- **`description`**: Acts as **routing logic** -- tells the agent runtime WHEN to load this skill. Not marketing copy; it describes triggers and exclusions.

The `references/` subdirectory contains detailed supplementary material loaded on demand by the agent.

### 3.3 Injection Mechanism: Claude Adapter (Primary)

The Claude Local adapter (`packages/adapters/claude-local/src/server/execute.ts`) uses a tmpdir + symlink pattern:

```
1. Resolve skills directory (relative to adapter package)
2. mkdtemp("paperclip-skills-") -> /tmp/paperclip-skills-XXXX
3. mkdir /tmp/paperclip-skills-XXXX/.claude/skills/
4. For each skill subdirectory:
     symlink(skills/<name>, /tmp/paperclip-skills-XXXX/.claude/skills/<name>)
5. Pass --add-dir /tmp/paperclip-skills-XXXX to Claude CLI
6. Agent discovers .claude/skills/ and loads skill metadata
7. Cleanup: rm -rf /tmp/paperclip-skills-XXXX in finally block
```

This pattern:
- Never pollutes the agent's working directory
- No git contamination of the project repo
- Each run gets a fresh, isolated skills mount
- Skills are universally available to all agents (no per-agent skill config)

### 3.4 Injection Mechanism: Codex Adapter (Alternative)

The Codex adapter uses a different approach -- global config directory injection:

```
1. Resolve CODEX_HOME (env var or ~/.codex)
2. For each skill not yet present:
     symlink(skills/<name>, $CODEX_HOME/skills/<name>)
3. Codex discovers skills from its global config
```

Idempotent -- existing symlinks are skipped.

### 3.5 Skill Resolution at Runtime

Skills are **NOT** configured per agent. The schema has no skills field on the `agents` table. Instead:

1. **All skills are globally available** to every agent via the injection mechanism
2. **Agent runtime discovers skills** via filesystem convention (`.claude/skills/`)
3. **Frontmatter description acts as routing logic** -- the agent decides which skills to load based on its current task context
4. **On-demand loading** -- agent sees skill metadata first, loads full content only when needed
5. **Session continuity** -- skill state persists across heartbeats via the task session system

This design means new skills become available to all agents without reconfiguration.

---

## 4. State Machines

### 4.1 Agent States (7 states)

```
                    ┌──────────┐
            ┌───────│   idle   │◄──────────────┐
            │       └────┬─────┘               │
            │            │ run starts          │ run succeeds/cancelled
            │            v                     │ (no more queued)
            │       ┌──────────┐               │
            │       │ running  │───────────────┘
            │       └────┬─────┘
            │            │ run fails (no more queued)
            │            v
            │       ┌──────────┐
            │       │  error   │
            │       └──────────┘
            │
  board pause│   board resume
            v            │
       ┌──────────┐      │
       │  paused  │◄─────┘
       └──────────┘
            │
  board terminate
            v
       ┌──────────────┐
       │  terminated   │  (terminal, irreversible)
       └──────────────┘

  Separate entry:
       ┌───────────────────┐
       │ pending_approval   │ ──board approve──> idle
       │ (hire gate)        │ ──board reject───> terminated
       └───────────────────┘
```

| State | Description | Can receive wakeups? |
|-------|-------------|---------------------|
| `idle` | Ready, no active run | Yes |
| `active` | Ready (legacy/manual) | Yes |
| `running` | Executing a heartbeat | Yes (coalesced) |
| `paused` | Manually or budget-paused | No |
| `error` | Last run failed | Yes |
| `pending_approval` | Awaiting board hire approval | No |
| `terminated` | Permanently deactivated | No (terminal) |

### 4.2 Issue States (7 states)

```
  ┌──────────┐    assign/     ┌──────────┐
  │ backlog  │───prioritize──>│   todo   │
  └──────────┘                └────┬─────┘
                                   │ checkout
                                   v
                              ┌──────────────┐     ┌──────────┐
                              │ in_progress  │────>│ in_review│
                              └──────┬───────┘     └────┬─────┘
                                     │                   │
                          block      │ complete          │ complete
                            v        v                   v
                      ┌─────────┐  ┌──────┐         ┌──────┐
                      │ blocked │  │ done │         │ done │
                      └─────────┘  └──────┘         └──────┘
                                   ┌───────────┐
              any ──cancel──>      │ cancelled │
                                   └───────────┘
```

| State | Description | Side Effects |
|-------|-------------|--------------|
| `backlog` | Default, unscheduled | Initial state |
| `todo` | Ready for pickup | In agent's inbox |
| `in_progress` | Agent working (requires assignee) | Sets `startedAt`, holds checkout lock |
| `in_review` | Work complete, under review | -- |
| `done` | Completed | Sets `completedAt` |
| `blocked` | Waiting on dependency | Agent should comment with reason |
| `cancelled` | Abandoned | Sets `cancelledAt` |

### 4.3 Heartbeat Run States (6 states)

```
  ┌─────────┐     claim      ┌──────────┐
  │ queued  │───────────────>│ running  │
  └────┬────┘                └────┬─────┘
       │ cancel                   │
       v                          ├── exit 0, no error ──> succeeded
  ┌───────────┐                   ├── exit != 0 / error -> failed
  │ cancelled │                   ├── cancel request ────> cancelled
  └───────────┘                   └── timeout ──────────> timed_out
```

| State | Terminal? | Trigger |
|-------|-----------|---------|
| `queued` | No | Created by `enqueueWakeup()` |
| `running` | No | Claimed by `claimQueuedRun()` (atomic UPDATE) |
| `succeeded` | Yes | Adapter returns exit 0, no error |
| `failed` | Yes | Adapter error, non-zero exit, or process lost |
| `cancelled` | Yes | Manual cancel or system cancel |
| `timed_out` | Yes | Execution exceeded timeout (SIGTERM then SIGKILL) |

### 4.4 Wakeup Request States (8 states)

```
  ┌─────────┐
  │ queued  │──┬── policy skip ────────> skipped
  └────┬────┘  ├── same-task running ──> coalesced
       │       └── issue locked ───────> deferred_issue_execution
       │                                        │
       │ run claims                              │ agent becomes invokable
       v                                         v
  ┌──────────┐                              ┌─────────┐
  │ claimed  │                              │ queued  │ (promoted)
  └────┬─────┘                              └─────────┘
       │ run finishes
       ├── succeeded ──> completed
       ├── failed ─────> failed
       └── cancelled ──> cancelled
```

### 4.5 Approval States (5 states)

```
  ┌──────────┐     request revision     ┌─────────────────────┐
  │ pending  │◄────────────────────────│ revision_requested  │
  └────┬─────┘     resubmit            └─────────────────────┘
       │                                        │
       ├── board approve ──> approved            ├── board approve ──> approved
       └── board reject ───> rejected            └── board reject ───> rejected
```

| State | Who transitions | Side effect |
|-------|----------------|-------------|
| `pending` | System (on create) | -- |
| `revision_requested` | Board | Requesting agent can resubmit |
| `approved` | Board | If hire_agent: creates/activates agent, wakes requester |
| `rejected` | Board | If hire_agent: terminates pending agent, wakes requester |
| `cancelled` | System/Board | -- |

### 4.6 Other Entity States

| Entity | States | Initial | Terminal |
|--------|--------|---------|----------|
| Company | active, paused, archived | active | (none enforced) |
| Goal | planned, active, achieved, cancelled | planned | achieved, cancelled |
| Project | backlog, planned, in_progress, completed, cancelled | backlog | completed, cancelled |
| Membership | pending, active, suspended | active | (none enforced) |
| Join Request | pending_approval, approved, rejected | pending_approval | approved, rejected |

---

## 5. Cross-Cutting Patterns

### 5.1 Company-Level Isolation

Every query is scoped by `companyId`. Cross-company access is impossible at the data layer. Within a company, there are no team-level boundaries -- all agents can see and be assigned any issue.

### 5.2 Actor-Based Audit Trail

The `activity_log` table records every mutation with `actorType` (agent/user/system), `actorId`, and `action` (dot-notation like `issue.created`, `agent.paused`). All entries carry `companyId` and optional `runId` for heartbeat traceability.

### 5.3 Atomic Operations

Critical operations use database-level atomicity:
- **Issue checkout**: Single UPDATE with compound WHERE clause
- **Budget enforcement**: `SET spent = spent + cost` atomic increment
- **Run claiming**: `UPDATE WHERE status='queued'` prevents double-claim
- **Issue execution lock**: `executionRunId` column with `FOR UPDATE` row locking

### 5.4 Session Continuity

Sessions persist across heartbeats via `agent_task_sessions` keyed on `(agentId, adapterType, taskKey)`. The adapter's `sessionCodec` handles serialization. Claude adapter stores `{ sessionId, cwd, workspaceId }` to enable conversation resume via `--resume`.

---

## 6. Key Takeaways for Praetor

| Paperclip Pattern | Relevance to Praetor |
|-------------------|---------------------|
| Issues as sole communication channel | Praetor uses plan steps and review cycles; no persistent task board |
| Sub-issue delegation chains | Praetor delegates via plan decomposition, not issue trees |
| Atomic checkout lock | Praetor uses git worktree isolation instead of DB locks |
| Skill injection via tmpdir symlinks | Praetor could adopt similar pattern for injecting tool instructions |
| Universal skill availability | Skills not configured per-agent; simplifies management |
| Heartbeat procedure in SKILL.md | Codifies agent behavior; Praetor's system prompt serves similar role |
| Environment variables as context | Praetor passes context via system prompt and CLI args |
| 7-state issue lifecycle | Praetor's simpler pass/fail model could benefit from richer states |
