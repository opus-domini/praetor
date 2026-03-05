# Paperclip: Codebase Structure, Technology Stack, and Business Model

Research output for TASK-001. Source: https://github.com/paperclipai/paperclip

---

## 1. What Paperclip Is

Paperclip is a **control plane for autonomous AI companies**. It orchestrates multiple AI agents into corporate structures with org charts, budgets, task hierarchies, and human governance. It is **not** an agent runtime — it coordinates agents that run elsewhere.

The core metaphor: "If OpenClaw is an _employee_, Paperclip is the _company_."

One Paperclip instance manages multiple **companies**, each with its own agents, org chart, budgets, and task hierarchy. Every piece of work traces back to a company-level goal.

---

## 2. Monorepo Structure

**Package manager:** pnpm 9.15.4 with workspaces
**Default branch:** `master`

```
paperclip/
├── cli/                          # CLI tool (npm: paperclipai)
│   └── src/
│       ├── commands/             # onboard, run, doctor, configure, client/*
│       ├── adapters/             # CLI-side adapter format (http, process)
│       ├── checks/               # Health checks (db, llm, port, secrets, storage)
│       ├── config/               # Config schema, env, secrets, data-dir
│       ├── prompts/              # Interactive setup prompts
│       └── utils/
├── server/                       # Express REST API (@paperclipai/server)
│   └── src/
│       ├── routes/               # REST endpoints (agents, issues, companies, costs, etc.)
│       ├── services/             # Business logic layer
│       ├── adapters/             # Server-side adapter execution (http, process)
│       ├── middleware/           # Auth, validation, error handling, board-mutation-guard
│       ├── auth/                 # Better Auth integration
│       ├── secrets/              # Encrypted secrets provider
│       ├── storage/              # Local disk + S3 asset storage
│       └── realtime/             # WebSocket live events
├── ui/                           # React SPA (@paperclipai/ui)
│   └── src/
│       ├── pages/                # Dashboard, Agents, Issues, Projects, Goals, Costs, etc.
│       ├── components/           # Sidebar, KanbanBoard, AgentConfigForm, ApprovalCard, etc.
│       ├── adapters/             # UI-side adapter config fields and transcript rendering
│       ├── api/                  # TanStack Query API hooks
│       ├── context/              # React contexts (Company, Theme, LiveUpdates, Panel, etc.)
│       └── lib/                  # Utils, router, query keys
├── packages/
│   ├── db/                       # Database layer (@paperclipai/db)
│   │   └── src/
│   │       ├── schema/           # Drizzle ORM schema (30+ tables)
│   │       └── migrations/       # 24 SQL migrations (0000–0023)
│   ├── shared/                   # Shared types & validators (@paperclipai/shared)
│   │   └── src/
│   │       ├── types/            # TypeScript types (agent, issue, company, cost, etc.)
│   │       └── validators/       # Zod validators
│   ├── adapter-utils/            # Shared adapter utilities (@paperclipai/adapter-utils)
│   └── adapters/                 # Agent execution adapters
│       ├── claude-local/         # Claude Code local process adapter
│       ├── codex-local/          # OpenAI Codex local process adapter
│       ├── cursor-local/         # Cursor editor adapter
│       ├── openclaw/             # OpenClaw HTTP adapter
│       └── opencode-local/       # OpenCode local process adapter
├── skills/                       # Agent skill definitions (SKILL.md files)
│   ├── paperclip/                # Core Paperclip API skill for agents
│   ├── paperclip-create-agent/   # Agent creation skill
│   ├── create-agent-adapter/     # Adapter authoring skill
│   └── para-memory-files/        # Memory file management skill
├── doc/                          # Internal specs and plans
│   ├── SPEC.md                   # Full technical specification
│   ├── PRODUCT.md                # Product definition
│   ├── GOAL.md                   # Vision statement
│   ├── CLIPHUB.md                # ClipHub marketplace spec
│   └── plans/                    # Implementation plans
├── docs/                         # Published documentation (Mintlify)
│   ├── start/                    # Quickstart, concepts, architecture
│   ├── guides/                   # Agent developer & board operator guides
│   ├── api/                      # REST API reference
│   ├── adapters/                 # Adapter documentation
│   ├── deploy/                   # Deployment guides
│   └── cli/                      # CLI reference
├── docker/                       # Docker support files
├── scripts/                      # Build, release, smoke test scripts
├── .changeset/                   # Changeset versioning
├── package.json                  # Root workspace config
├── pnpm-workspace.yaml           # Workspace definition
├── docker-compose.yml            # Docker composition
├── Dockerfile                    # Main Dockerfile
└── AGENTS.md                     # Agent coding guidelines
```

### Workspace Packages (pnpm-workspace.yaml)

```yaml
packages:
  - packages/*
  - packages/adapters/*
  - server
  - ui
  - cli
```

### Published npm Packages

| Package | npm name | Version |
|---------|----------|---------|
| CLI | `paperclipai` | 0.2.7 |
| Server | `@paperclipai/server` | 0.2.7 |
| Database | `@paperclipai/db` | 0.2.7 |
| Shared | `@paperclipai/shared` | 0.2.7 |
| Adapter Utils | `@paperclipai/adapter-utils` | published |
| Claude Local | `@paperclipai/adapter-claude-local` | published |
| Codex Local | `@paperclipai/adapter-codex-local` | published |
| Cursor Local | `@paperclipai/adapter-cursor-local` | published |
| OpenClaw | `@paperclipai/adapter-openclaw` | published |
| OpenCode Local | `@paperclipai/adapter-opencode-local` | published |

---

## 3. Technology Stack

### Backend

| Component | Technology | Version |
|-----------|-----------|---------|
| Runtime | Node.js | >= 20 |
| Language | TypeScript | ^5.7.3 |
| HTTP Framework | Express.js | ^5.1.0 |
| Database ORM | Drizzle ORM | ^0.38.4 |
| Database | PostgreSQL 17 (prod) / PGlite embedded (dev) | — |
| Embedded PG | embedded-postgres | ^18.1.0-beta.16 |
| Auth | Better Auth | 1.4.18 |
| Validation | Zod | ^3.24.2 |
| WebSockets | ws | ^8.19.0 |
| Logging | pino + pino-http + pino-pretty | ^9.6.0 / ^10.4.0 / ^13.1.3 |
| File uploads | multer | ^2.0.2 |
| Object storage | @aws-sdk/client-s3 | ^3.888.0 |
| Env vars | dotenv | ^17.0.1 |

### Frontend

| Component | Technology | Version |
|-----------|-----------|---------|
| Framework | React | ^19.0.0 |
| Build tool | Vite | ^6.1.0 |
| Router | react-router-dom | ^7.1.5 |
| Data fetching | @tanstack/react-query | ^5.90.21 |
| CSS | Tailwind CSS | ^4.0.7 |
| UI primitives | Radix UI | ^1.4.3 |
| Icons | lucide-react | ^0.574.0 |
| Markdown | react-markdown + remark-gfm | ^10.1.0 / ^4.0.1 |
| Rich editor | @mdxeditor/editor | ^3.52.4 |
| Drag and drop | @dnd-kit/core + @dnd-kit/sortable | ^6.3.1 / ^10.0.0 |
| Command palette | cmdk | ^1.1.1 |
| Class utils | clsx + tailwind-merge + class-variance-authority | latest |

### CLI

| Component | Technology | Version |
|-----------|-----------|---------|
| Command framework | commander | ^13.1.0 |
| Interactive prompts | @clack/prompts | ^0.10.0 |
| Colors | picocolors | ^1.1.1 |
| Build | esbuild | ^0.27.3 |

### Dev Tooling

| Tool | Version |
|------|---------|
| Package manager | pnpm 9.15.4 |
| Test framework | vitest ^3.0.5 |
| TypeScript compiler | ^5.7.3 |
| HTTP testing | supertest ^7.0.0 |
| Dev runner | tsx ^4.19.2 |
| Versioning | @changesets/cli ^2.30.0 |

### Documentation

Published docs use **Mintlify** (`docs/docs.json` config, `npx mintlify dev`).

---

## 4. Architecture Overview

### Layered Architecture

```
┌─────────────────────────────────┐
│         React UI (Vite)         │  Browser SPA
├─────────────────────────────────┤
│      Express REST API           │  Single unified API for UI + agents
│  (routes → services → db)      │
├─────────────────────────────────┤
│    PostgreSQL (Drizzle ORM)     │  30+ tables, 24 migrations
├─────────────────────────────────┤
│     Pluggable Adapters          │  Agent execution layer
│  (claude, codex, cursor,       │
│   openclaw, opencode, http)     │
└─────────────────────────────────┘
```

### Key Architectural Decisions

- **Single unified REST API** — same endpoints serve both the UI and agents; auth determines permissions
- **Adapter pattern** — each adapter implements `execute()`, `status()`, `cancel()` for agent lifecycle
- **Atomic task checkout** — single-assignee model with database-enforced atomic claiming
- **Company-level isolation** — one instance runs many companies with complete data separation
- **Heartbeat system** — agents wake on schedules, not continuous; Paperclip controls when/how, agents control what
- **No auto-recovery** — stale tasks are surfaced to humans, never silently reassigned
- **WebSocket realtime** — live UI updates via `ws` library

### Database Schema (30+ tables)

Key entities: companies, agents, issues, projects, goals, approvals, cost_events, activity_log, heartbeat_runs, company_secrets, agent_api_keys, agent_config_revisions, agent_runtime_state, agent_task_sessions, labels, assets, invites, join_requests, instance_user_roles, principal_permission_grants.

### Adapter System

Each adapter has three modules:
1. **Server** — `execute()` spawns/calls the agent, `parse()` handles output, `test()` validates config
2. **UI** — `build-config.ts` generates config, `parse-stdout.ts` processes output for display
3. **CLI** — `format-event.ts` formats events for terminal output

Built-in adapters: `claude-local`, `codex-local`, `cursor-local`, `openclaw`, `opencode-local`, `http`, `process`.

### Agent Integration Levels

1. **Callable** (minimum) — Paperclip can invoke the agent
2. **Status reporting** — agent reports success/failure
3. **Fully instrumented** — agent reports status, costs, task updates, logs (bidirectional)

---

## 5. Deployment Model

### Two Runtime Modes

| Mode | Description |
|------|-------------|
| `local_trusted` | Single-user, loopback-only, no login friction. Default. |
| `authenticated` | Login required. Sub-modes: `private` (LAN/VPN) or `public` (internet-facing). |

### Progressive Deployment Path

1. **Local dev** — `npx paperclipai onboard --yes`. Embedded PGlite, everything local.
2. **Hosted** — Deploy to any Node.js host. External PostgreSQL. Remote agents connect via API keys.
3. **Open company** — Optionally expose parts publicly (future).

### Docker Support

- `docker-compose.yml` and `docker-compose.quickstart.yml` provided
- Main `Dockerfile` and smoke-test Dockerfile (`Dockerfile.onboard-smoke`)

### Agent Authentication

1. Human creates agent in UI
2. Paperclip generates a **connection string** (URL + API key + instructions)
3. Human provides string to agent's environment
4. Agent authenticates API calls with the key

---

## 6. Business Model

### Current State: Fully Open Source

- **License:** MIT
- **Repository:** Public on GitHub
- **npm packages:** Published publicly under `@paperclipai/*` scope
- **Self-hosted:** Single-tenant, not a SaaS
- **No paid tiers** currently

### Planned Monetization: ClipHub (Company Marketplace)

ClipHub is specified in `doc/CLIPHUB.md` as a **separate hosted service** — a public registry for sharing and downloading Paperclip company templates. Analogous to npm for packages or Docker Hub for containers.

**Key ClipHub concepts:**
- **Company templates** — exportable org configs (agents, hierarchy, adapter configs, seed tasks, budgets)
- **Sub-packages** — individual agent templates, team templates, adapter configs
- **Semantic search** — vector embeddings for intent-based discovery
- **Publishing** — `paperclipai publish cliphub <name>` from CLI
- **Install** — `paperclipai install cliphub:<publisher>/<slug>`
- **Forking** — copy and modify templates, with lineage tracking
- **Stars, comments, download counts** — community signals
- **GitHub OAuth** — authentication for publishers
- **Verified publishers** — trust badges for quality signals

**ClipHub V1 scope:** Publishing, browsing, search, install via CLI, stars, downloads, versioning, basic moderation.

**Not in initial scope:** Paid/premium templates, private registries, running companies on ClipHub.

**Monetization path (inferred):**
- ClipHub is a **hosted service** while Paperclip itself is self-hosted
- Future enterprise features: private registries, premium templates, hosted Paperclip instances
- The registry creates a network effect: more templates → more users → more templates

### Revenue Anti-Goals

- External revenue/expense tracking is explicitly a future plugin, not core
- Token/LLM cost **budgeting** is core, but actual billing is not

---

## 7. Target Audience

### Primary: AI-First Operators

People who want to build and run "autonomous AI companies" — organizations where all employees are AI agents, coordinated through a structured control plane with human board oversight.

### Secondary: Agent Developers

Developers building AI agents who need a control plane for orchestration, task management, cost tracking, and multi-agent coordination.

### Tertiary: Template Authors (ClipHub)

Users who build and share reusable company configurations — "download a company" is the pitch.

### Use Case Examples (from ClipHub categories)

Software development shops, marketing agencies, content studios, research firms, operations teams, sales teams, finance/legal services, creative agencies.

---

## 8. Competitive Positioning

### What Paperclip IS

- Control plane for AI agent orchestration
- Corporate structure layer (org charts, governance, budgets)
- Self-hosted, single-tenant infrastructure
- Agent-runtime agnostic

### What Paperclip IS NOT (explicit anti-requirements)

- Not an agent runtime
- Not a knowledge base (future plugin territory)
- Not a SaaS
- Not opinionated about agent implementation
- Not automatically self-healing
- Does not manage work artifacts (repos, deployments, files)
- Does not auto-reassign work
- Does not track external revenue/expenses

---

## 9. Key Differences from Praetor

| Dimension | Praetor | Paperclip |
|-----------|---------|-----------|
| Language | Go | TypeScript (Node.js) |
| Scope | Single-task orchestrator | Multi-company control plane |
| Agent model | Executor/reviewer pipeline | Corporate hierarchy (CEO → teams) |
| Task model | Plan-driven sequential execution | Hierarchical task board (goals → projects → issues) |
| Governance | Automated fallback + review | Human board approval gates |
| Deployment | CLI tool | Web server + React UI + CLI |
| Database | None (file-based) | PostgreSQL with Drizzle ORM |
| Concurrency | Worktree isolation per task | Atomic task checkout per agent |
| Agent support | 9 providers (Claude, Codex, etc.) | 5 adapters (Claude, Codex, Cursor, OpenClaw, OpenCode) |
| Business model | — | Open source + ClipHub marketplace |
