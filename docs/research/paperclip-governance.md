# Paperclip Governance: Budget Enforcement, Approvals, and Organizational Hierarchy

Research output for TASK-003. Source: https://github.com/paperclipai/paperclip

---

## 1. Budget Enforcement

Paperclip implements a 2-tier budget system where both the company and each individual agent carry monthly spending limits denominated in cents.

### 1.1 Two-Tier Budget Schema

**Company-level** (`companies` table):
- `budgetMonthlyCents` (integer, default 0) -- the ceiling
- `spentMonthlyCents` (integer, default 0) -- running accumulator

**Agent-level** (`agents` table):
- `budgetMonthlyCents` (integer, default 0)
- `spentMonthlyCents` (integer, default 0)

Both tiers are incremented atomically when a cost event is recorded. A budget of 0 means "unlimited" -- the auto-pause check only fires when `budgetMonthlyCents > 0`.

### 1.2 Cost Event Recording

Every LLM invocation is reported via `POST /api/companies/{companyId}/cost-events`. The payload:

```typescript
{
  agentId: string,           // required -- who incurred the cost
  issueId?: string,          // optional -- traceable to work item
  projectId?: string,
  goalId?: string,
  billingCode?: string,      // arbitrary tag for cost allocation
  provider: string,          // e.g. "anthropic", "openai"
  model: string,             // e.g. "claude-sonnet-4-20250514"
  inputTokens: number,
  outputTokens: number,
  costCents: number,         // the amount in cents
  occurredAt: string         // ISO 8601
}
```

**Processing flow** (`server/src/services/costs.ts`):

1. Validate event and verify agent belongs to the company
2. Insert row into `cost_events` table
3. Atomically increment both `agents.spentMonthlyCents` and `companies.spentMonthlyCents`:
   ```sql
   SET spent_monthly_cents = spent_monthly_cents + cost_cents
   ```
4. Check agent budget threshold (see below)
5. Log activity (`cost.reported` / `cost.recorded`)

The `cost_events` table carries two indexes optimized for time-range queries:
- `(companyId, occurredAt)` -- company-level summaries
- `(companyId, agentId, occurredAt)` -- per-agent breakdowns

### 1.3 Threshold Behavior

| Utilization | Level | Behavior |
|-------------|-------|----------|
| < 80% | Normal | No intervention |
| >= 80% | **Soft alert** | Informational only. Agents are expected to self-check via `GET /api/agents/me` and voluntarily focus on critical tasks. The system does **not** enforce this automatically. |
| >= 100% | **Hard ceiling** | Agent is **auto-paused** by the system. No new heartbeats fire. |

**Auto-pause logic** (triggered inside cost event creation):

```typescript
if (
  updatedAgent.budgetMonthlyCents > 0 &&
  updatedAgent.spentMonthlyCents >= updatedAgent.budgetMonthlyCents &&
  updatedAgent.status !== "paused" &&
  updatedAgent.status !== "terminated"
) {
  await db.update(agents)
    .set({ status: "paused", updatedAt: new Date() })
    .where(eq(agents.id, updatedAgent.id));
}
```

The check fires at the agent level only. Company-level budget exhaustion is surfaced through the dashboard but does not auto-pause agents (a company may have agents with individual budgets well under the company ceiling).

### 1.4 Budget Window and Reset

The spec states budget windows reset on the first of each month (UTC). However, the codebase has **no automatic monthly reset** of `spentMonthlyCents`. The dashboard dynamically calculates current-month spend by filtering `cost_events` on `occurredAt >= monthStart`, so the cached counters may drift from the real-time view. Reset appears to require an external cron job or manual board action.

### 1.5 Budget Management Endpoints

| Endpoint | Who | What |
|----------|-----|------|
| `PATCH /companies/{id}/budgets` | Board only | Set company `budgetMonthlyCents` |
| `PATCH /agents/{id}/budgets` | Board or owning agent | Set agent `budgetMonthlyCents` |
| `GET /companies/{id}/costs/summary` | Company access | Company-level spend + utilization % |
| `GET /companies/{id}/costs/by-agent` | Company access | Per-agent breakdown |
| `GET /companies/{id}/costs/by-project` | Company access | Per-project breakdown |

### 1.6 Recovery from Auto-Pause

- Board manually resumes via `POST /api/agents/:id/resume` (status -> `idle`)
- Or wait for the next calendar month (if external reset is configured)

---

## 2. Approval System

Approvals are the primary governance gate. Certain high-impact actions require explicit board sign-off before taking effect.

### 2.1 Approval Types

Two approval types exist:

| Type | Purpose | Triggered by |
|------|---------|-------------|
| `hire_agent` | Gate new agent creation | CEO (or agent with `canCreateAgents`) requesting a subordinate |
| `approve_ceo_strategy` | Gate CEO's initial strategic task breakdown | CEO on first heartbeat |

### 2.2 Approval Schema

**`approvals` table:**
- `id` (UUID), `companyId`, `type`, `status` (default `pending`)
- `requestedByAgentId` / `requestedByUserId` -- who initiated
- `payload` (JSONB) -- type-specific data (e.g. agent config for `hire_agent`)
- `decisionNote`, `decidedByUserId`, `decidedAt` -- resolution metadata

**`approval_comments` table:** Discussion thread per approval.

**`issue_approvals` table:** Links approvals to issues for traceability.

### 2.3 Approval Lifecycle

```
                        ┌──────────────────┐
                        │     pending       │
                        └────┬────┬────┬───┘
                 approve │   │    │    │ reject
                         v   │    │    v
                  ┌──────────┐  │  ┌──────────┐
                  │ approved  │  │  │ rejected  │
                  │ (terminal)│  │  │ (terminal)│
                  └──────────┘  │  └──────────┘
                                │
                  request_revision
                                │
                                v
                  ┌─────────────────────┐
                  │ revision_requested  │
                  └────────┬────────────┘
                           │ resubmit (requester only)
                           v
                  ┌──────────────────┐
                  │    pending       │  (cycle back)
                  └──────────────────┘
```

**Status values:** `pending`, `revision_requested`, `approved`, `rejected`, `cancelled`

### 2.4 Approval Resolution Effects

**On approve (`hire_agent`):**
1. If `payload.agentId` exists: activate the pending-approval agent
2. Otherwise: create a new agent with the payload configuration
3. Wake the requesting agent via `heartbeat.wakeup()` with context:
   - `reason: "approval_approved"`
   - `source: "automation"`, `triggerDetail: "system"`
   - Payload includes `approvalId`, `approvalStatus`, `issueId`, `issueIds`

**On reject (`hire_agent`):**
1. Terminate the pending agent (if one was pre-created)
2. Wake the requesting agent with `reason: "approval_rejected"`

**On request revision:**
1. Status moves to `revision_requested`
2. Requesting agent can resubmit with updated payload (returns to `pending`)

### 2.5 Approval Authorization

| Action | Who can perform |
|--------|----------------|
| Create approval | Any principal with company access |
| Approve / Reject / Request revision | **Board only** (`assertBoard()`) |
| Resubmit | Only the original requesting agent/user |
| Comment | Any principal with company access |
| View | Any principal with company access |

### 2.6 Secret Normalization

For `hire_agent` payloads that contain adapter secrets (API keys, tokens), the system runs `secretsSvc.normalizeHireApprovalPayloadForPersistence()` to sanitize secrets before database storage. Strict mode is available via `PAPERCLIP_SECRETS_STRICT_MODE`.

---

## 3. Activity Logging

Every significant mutation in the system is recorded to the `activity_log` table, providing a complete audit trail.

### 3.1 Activity Log Schema

```typescript
{
  id: UUID,
  companyId: UUID,          // scoped to company
  actorType: text,          // "agent" | "user" | "system"
  actorId: text,            // agent ID, user ID, or system identifier
  action: text,             // dot-notation action key
  entityType: text,         // what was affected
  entityId: text,           // ID of affected entity
  agentId?: UUID,           // optional -- agent involved
  runId?: UUID,             // optional -- heartbeat run context
  details?: JSONB,          // freeform additional context
  createdAt: timestamp
}
```

Indexes: `(companyId, createdAt)`, `(runId)`, `(entityType, entityId)`.

### 3.2 Actor Types

| actorType | Source | Description |
|-----------|--------|-------------|
| `agent` | API key / JWT auth | AI agent performing an action. `actorId` = agent UUID. |
| `user` | Session / local-implicit auth | Human board operator. `actorId` = user ID or `"board"`. |
| `system` | Internal services | System-generated actions (default). `actorId` = service name. |

Actor resolution from HTTP request (`server/src/routes/authz.ts`):
```typescript
function getActorInfo(req: Request) {
  if (req.actor.type === "agent") {
    return { actorType: "agent", actorId: req.actor.agentId, agentId: req.actor.agentId, runId: req.actor.runId };
  }
  return { actorType: "user", actorId: req.actor.userId ?? "board", agentId: null, runId: req.actor.runId };
}
```

### 3.3 Logged Actions by Category

| Category | Actions |
|----------|---------|
| **Issues** | `issue.created`, `issue.updated`, `issue.checked_out`, `issue.released`, `issue.comment_added`, `issue.attachment_added`, `issue.attachment_removed`, `issue.deleted` |
| **Agents** | `agent.created`, `agent.updated`, `agent.paused`, `agent.resumed`, `agent.terminated`, `agent.key_created`, `agent.budget_updated`, `agent.runtime_session_reset` |
| **Approvals** | `approval.created`, `approval.approved`, `approval.rejected`, `approval.revision_requested`, `approval.resubmitted`, `approval.comment_added` |
| **Heartbeats** | `heartbeat.invoked`, `heartbeat.cancelled` |
| **Projects** | `project.created`, `project.updated`, `project.deleted` |
| **Goals** | `goal.created`, `goal.updated`, `goal.deleted` |
| **Costs** | `cost.reported`, `cost.recorded` |
| **Company** | `company.created`, `company.updated`, `company.archived`, `company.budget_updated` |

### 3.4 Live Events

Activity log entries also publish WebSocket `activity.logged` events for real-time UI updates.

---

## 4. Organizational Hierarchy

Paperclip models companies as tree-structured organizations where AI agents occupy named roles in a reporting hierarchy.

### 4.1 Hierarchy Model

The hierarchy is implemented via a self-referential foreign key on the `agents` table:

```sql
reports_to UUID REFERENCES agents(id)
```

- Agents with `reportsTo = null` are root-level (typically the CEO)
- The relationship creates a single-rooted tree per company
- Indexed via `(companyId, reportsTo)` for efficient subtree queries

### 4.2 Agent Roles

11 predefined roles:

| Role | Typical Position | Special Permissions |
|------|-----------------|-------------------|
| `ceo` | Top-level strategic agent | `canCreateAgents: true` (only role with this default) |
| `cto` | Technical leadership | — |
| `cmo` | Marketing leadership | — |
| `cfo` | Financial leadership | — |
| `engineer` | Software development | — |
| `designer` | Creative / design | — |
| `pm` | Product management | — |
| `qa` | Quality assurance | — |
| `devops` | Infrastructure / ops | — |
| `researcher` | Research | — |
| `general` | Default / generic | — |

### 4.3 Agent Statuses

```
active          -- ready to receive work
idle            -- active but no current heartbeat
running         -- executing a heartbeat
paused          -- manually paused or budget-paused
error           -- last heartbeat failed
pending_approval -- awaiting board approval (hire gate)
terminated      -- permanently deactivated (irreversible)
```

Agents in `paused`, `terminated`, or `pending_approval` reject all wakeups.

### 4.4 Org Tree Operations

**Build full tree** (`agentService.orgForCompany`):
- Fetches all agents for a company
- Groups by `reportsTo` into a `Map<managerId, agents[]>`
- Recursively builds tree from `null` root
- Exposed via `GET /companies/:companyId/org`

**Chain of command** (`agentService.getChainOfCommand`):
- Walks `reportsTo` links upward from a given agent
- Returns array of `{ id, name, role, title }` up to root
- Cycle-safe with visited set and 50-hop limit

**Cycle prevention** (`assertNoCycle`):
- Before any `reportsTo` update, walks the chain from the proposed manager upward
- Rejects if it would reach back to the agent being updated
- Also rejects self-reference (`reportsTo === agentId`)

### 4.5 UI Visualization

The org chart (`ui/src/pages/OrgChart.tsx`) renders an SVG tree:
- Cards: 200px x 100px per agent
- Horizontal gap: 32px, vertical gap: 80px
- Color-coded status dots on each card
- Recursive layout algorithm computing subtree widths

---

## 5. The Board Operator Model

The Board is the human governance layer. In Paperclip's metaphor, board members are the only humans -- everything else is an AI agent.

### 5.1 Actor Types in the System

| Actor Type | Authentication | Powers |
|------------|---------------|--------|
| `board` (human) | Session auth or `local_implicit` | Full governance: approve/reject, pause/resume, budget control, override any agent decision |
| `agent` | API key or local JWT | Scoped to own company, own work. Can request approvals. |
| `none` | Unauthenticated | No access |

### 5.2 Board Authentication Modes

**Local-trusted mode** (`local_implicit`):
- All local requests automatically get `type: "board"` with `isInstanceAdmin: true`
- No login friction for single-user setups

**Authenticated mode** (`session`):
- Better Auth session required
- User's `instance_user_roles` checked for `instance_admin` (short-circuits all authz)
- User's `company_memberships` determine which companies they can access

### 5.3 RBAC: Two-Layer Authorization

**Layer 1: Instance-level**
- Table: `instance_user_roles`
- Single role: `instance_admin`
- Short-circuits all company-level checks

**Layer 2: Company-level**
- Table: `principal_permission_grants`
- Keyed on `(companyId, principalType, principalId, permissionKey)`
- Permission keys: `agents:create`, `users:invite`, `users:manage_permissions`, `tasks:assign`, `tasks:assign_scope`, `joins:approve`
- Optional `scope` (JSONB) for fine-grained constraints

**Company membership** (`company_memberships` table):
- Both users and agents are "principals"
- Status: `pending`, `active`, `suspended`
- Required `active` status for any company access

### 5.4 Board-Mutation Guard

The `board-mutation-guard` middleware prevents CSRF on board write operations in cloud mode:
- Safe methods (`GET`, `HEAD`, `OPTIONS`) always pass
- Non-board actors skip the guard
- `local_implicit` source exempted (no browser involved)
- Cloud mode: validates `Origin` or `Referer` header against trusted origins

### 5.5 Board Powers Summary

| Power | Mechanism |
|-------|-----------|
| Set company/agent budgets | `PATCH /companies/{id}/budgets`, `PATCH /agents/{id}/budgets` |
| Pause/resume any agent | `POST /agents/{id}/pause`, `POST /agents/{id}/resume` |
| Terminate agent | `POST /agents/{id}/terminate` |
| Approve/reject approvals | `POST /approvals/{id}/approve`, `/reject`, `/request-revision` |
| Full project management | All issue/project/goal CRUD endpoints |
| Override agent decisions | Reassign tasks, change priorities, modify descriptions |
| View audit trail | `GET /companies/{id}/activity` |
| View cost analytics | Cost summary and breakdown endpoints |

---

## 6. Key Takeaways for Praetor

### What Paperclip does well in governance:

1. **Budget enforcement is simple and effective** -- atomic counter increments with a single comparison trigger auto-pause. No complex accounting.

2. **Approval gates are minimal** -- only 2 types (`hire_agent`, `approve_ceo_strategy`), keeping the governance surface small. The revision loop is elegant.

3. **Activity logging is pervasive** -- every mutation records actor type, enabling full auditability without complex event-sourcing.

4. **Org hierarchy is a simple tree** -- `reportsTo` foreign key with cycle prevention. No complex graph structures.

5. **Board-as-human is clean** -- clear separation between human oversight (board) and AI execution (agents). The actor type system makes this explicit.

### Differences from Praetor's model:

| Aspect | Paperclip | Praetor |
|--------|-----------|---------|
| Budget control | DB counters + auto-pause | No budget system |
| Approval gates | Explicit board approval flow | Automated review pipeline |
| Governance model | Human board governs AI agents | Automated fallback + human escalation |
| Audit trail | activity_log table with actor types | Plan-based execution log |
| Org structure | Tree hierarchy (CEO -> teams) | Flat executor/reviewer |
| Agent identity | Named roles with permissions | Generic provider adapters |
