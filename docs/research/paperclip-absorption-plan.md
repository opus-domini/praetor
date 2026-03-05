# Paperclip Absorption Plan for Praetor

Research output for TASK-008. Builds on TASK-001 through TASK-007.

---

## Principles of Absorption

These principles govern every task in this plan. They are non-negotiable constraints.

1. **No new dependencies.** All implementations use Go stdlib (`os`, `encoding/json`, `crypto/sha256`, `sync`). No external packages.
2. **Filesystem is truth.** All state is file-based (JSON, JSONL, TSV). No databases, no in-memory-only state.
3. **Single binary.** No new services, daemons, or background processes. Everything compiles into the existing `praetor` binary.
4. **CLI-first.** All features are accessible through the existing Cobra command tree. No HTTP server, no WebSocket, no React UI.
5. **Extend, don't replace.** New features extend existing interfaces and schemas. Backward compatibility with existing plans and state is mandatory.
6. **Smallest viable change.** Each task is one pull request. No task depends on understanding the full plan -- only its declared dependencies.
7. **Test what you build.** Every task includes acceptance criteria that can be verified by running existing tests or new unit tests.

---

## Milestone Overview

| # | Milestone | Scope | Tasks |
|---|-----------|-------|-------|
| M1 | Budget Enforcement | Monetary cost tracking with per-plan ceilings and auto-cancel | TASK-001 to TASK-005 |
| M2 | Actor Tracking | Enrich events with actor/entity attribution for audit trails | TASK-006 to TASK-010 |
| M3 | Structured Feedback | Session persistence and cumulative agent statistics | TASK-011 to TASK-016 |
| M4 | Plan Templates | Reusable plan templates with parameterized variables | TASK-017 to TASK-021 |
| M5 | Richer Health Checks | Environment variable context injection and enriched probing | TASK-022 to TASK-025 |
| M6 | Parallel Execution | Concurrent task execution for independent DAG branches | TASK-026 to TASK-030 |

---

## Dependency Graph Between Milestones

```
M1 (Budget Enforcement)
  |
  v
M2 (Actor Tracking) -----> M3 (Structured Feedback)
                              |
                              v
M4 (Plan Templates)        M5 (Richer Health Checks)
                              |
                              v
                           M6 (Parallel Execution)
```

- **M1 is independent.** It can start immediately.
- **M2 depends on M1** -- actor-typed events reference cost events introduced in M1.
- **M3 depends on M2** -- session persistence emits actor-typed events.
- **M4 is independent.** It can run in parallel with M1-M3.
- **M5 depends on M3** -- environment variable injection uses session state from M3.
- **M6 depends on M3 and M5** -- parallel execution needs task locks (M3 foundation) and enriched probing (M5).

---

## 4-Phase Implementation Roadmap

| Phase | Milestones | Focus | Rationale |
|-------|-----------|-------|-----------|
| Phase 1: Cost Control | M1 | Monetary budget tracking, cost enrichment, cumulative stats | Prevents runaway spending. Highest ROI, lowest risk. |
| Phase 2: Observability | M2, M4 | Actor-typed events, activity attribution, plan templates | Improves debugging and reusability. M4 is independent work. |
| Phase 3: Continuity | M3, M5 | Session persistence, session codec, env var injection | Reduces retry cost, enables agent resume. |
| Phase 4: Concurrency | M6 | Task locks, parallel DAG walker, concurrent execution | Enables multi-task parallelism. Highest complexity, highest reward. |

**Phase overlap:** M4 (Plan Templates) can be developed in parallel with any phase since it has no dependencies on other milestones.

---

## Milestone 1: Budget Enforcement

**Goal:** Track actual LLM spending in cents, enrich cost data with provider/model metadata, add per-plan budget ceilings with auto-cancel, and accumulate cross-run statistics.

### TASK-001: Add monetary budget fields to ExecutionPolicy

**Target files:**
- `internal/domain/types.go`

**Description:** Extend `ExecutionPolicy` with an optional `BudgetCents` field (monetary ceiling per plan run) and `SpentCents` to the `State` struct for tracking accumulated spend.

**Code example:**
```go
// internal/domain/types.go

type ExecutionPolicy struct {
	MaxTotalIterations int          `json:"max_total_iterations,omitempty"`
	MaxRetriesPerTask  int          `json:"max_retries_per_task,omitempty"`
	Timeout            string       `json:"timeout,omitempty"`
	Budget             BudgetPolicy `json:"budget,omitempty"`
	StallDetection     StallPolicy  `json:"stall_detection,omitempty"`
	BudgetCents        int          `json:"budget_cents,omitempty"` // monetary ceiling in cents; 0 = unlimited
}

type State struct {
	PlanSlug     string      `json:"plan_slug"`
	PlanChecksum string      `json:"plan_checksum"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
	Outcome      RunOutcome  `json:"outcome,omitempty"`
	SpentCents   float64     `json:"spent_cents,omitempty"` // accumulated monetary cost
	Tasks        []StateTask `json:"tasks"`
}
```

**Acceptance criteria:**
- `ExecutionPolicy` has a `BudgetCents` field that deserializes from plan JSON
- `State` has a `SpentCents` field that serializes/deserializes correctly
- Existing plans without `budget_cents` continue to load (zero value = unlimited)
- `go test ./internal/domain/...` passes

---

### TASK-002: Add --budget-cents CLI flag and config key

**Target files:**
- `internal/cli/plan_run.go`
- `internal/config/config.go`
- `internal/domain/types.go` (RunnerOptions)

**Description:** Wire the `budget_cents` field through CLI flags and config, following the existing precedence pattern (CLI > config > plan settings > defaults).

**Code example:**
```go
// internal/domain/types.go - add to RunnerOptions
type RunnerOptions struct {
	// ... existing fields ...
	BudgetCents        int
	BudgetCentsSet     bool
}

// internal/cli/plan_run.go - add flag
cmd.Flags().IntVar(&opts.BudgetCents, "budget-cents", 0, "monetary budget ceiling in cents (0 = unlimited)")
```

**Acceptance criteria:**
- `praetor plan run my-plan --budget-cents 500` sets the monetary ceiling
- Config key `budget-cents` in `config.toml` is respected
- Plan settings `execution_policy.budget_cents` is respected
- Precedence order is CLI > config > plan > default (0)
- `go test ./internal/cli/...` passes

---

### TASK-003: Enrich cost tracking with provider and model fields

**Target files:**
- `internal/domain/types.go` (CostEntry)
- `internal/state/store_metrics.go`
- `internal/agent/middleware/events.go`

**Description:** Extend `CostEntry` with `Provider` and `Model` fields. Update the TSV header and writer to include these fields. Enrich the `ExecutionEvent` with `Model` field.

**Code example:**
```go
// internal/domain/types.go
type CostEntry struct {
	Timestamp string
	RunID     string
	TaskID    string
	Agent     string
	Role      string
	DurationS float64
	Status    string
	CostUSD   float64
	Provider  string  // new: e.g. "anthropic", "openai"
	Model     string  // new: e.g. "claude-sonnet-4-20250514"
}
```

```go
// internal/state/store_metrics.go - updated TSV format
// Header: timestamp\trun_id\ttask_id\tagent\trole\tduration_s\tstatus\tcost_usd\tprovider\tmodel
```

**Acceptance criteria:**
- `CostEntry` includes `Provider` and `Model` fields
- TSV output includes two new columns (provider, model)
- Existing TSV files without new columns remain readable (backward compatible)
- `ExecutionEvent` includes `Model` field
- `go test ./internal/state/...` and `go test ./internal/domain/...` pass

---

### TASK-004: Accumulate spent_cents in pipeline after each task

**Target files:**
- `internal/orchestration/pipeline/runner.go`

**Description:** After each executor/reviewer invocation, convert `CostUSD` from `AgentResponse` to cents and add to `State.SpentCents`. Persist the updated value in snapshots.

**Code example:**
```go
// internal/orchestration/pipeline/runner.go - after executor returns
func (r *activeRun) accumulateCost(costUSD float64) {
	cents := costUSD * 100
	r.state.SpentCents += cents
}
```

**Acceptance criteria:**
- `State.SpentCents` is incremented after each executor invocation
- `State.SpentCents` is incremented after each reviewer invocation
- Accumulated value is persisted in snapshot JSON
- Accumulated value survives state recovery from snapshot
- `go test ./internal/orchestration/pipeline/...` passes

---

### TASK-005: Enforce budget ceiling with auto-cancel

**Target files:**
- `internal/orchestration/pipeline/runner.go`

**Description:** Before each task execution, check if `State.SpentCents >= BudgetCents`. If exceeded, emit a `budget_exceeded` event and set `RunOutcome: canceled`. Use the existing FSM to halt the loop.

**Code example:**
```go
// internal/orchestration/pipeline/runner.go
func (r *activeRun) checkBudgetCeiling() bool {
	ceiling := r.plan.Settings.ExecutionPolicy.BudgetCents
	if ceiling <= 0 {
		return false // unlimited
	}
	return r.state.SpentCents >= float64(ceiling)
}
```

**Acceptance criteria:**
- Pipeline halts with `RunOutcome: canceled` when `SpentCents >= BudgetCents`
- A `budget_exceeded` event is emitted with spent and ceiling values
- Plans without `budget_cents` (or `budget_cents: 0`) run without limit
- Exit code is `2` (canceled) when budget is exceeded
- `go test ./internal/orchestration/pipeline/...` passes

---

## Milestone 2: Actor Tracking

**Goal:** Enrich the existing event schema with actor type and entity type fields, enabling forensic analysis of who/what caused each state transition.

### TASK-006: Define ActorType and EntityType constants

**Target files:**
- `internal/agent/middleware/events.go`

**Description:** Add `ActorType` and `EntityType` string types with constants. Actor types identify who caused a mutation. Entity types identify what was affected.

**Code example:**
```go
// internal/agent/middleware/events.go

type ActorType string

const (
	ActorExecutor ActorType = "executor"
	ActorReviewer ActorType = "reviewer"
	ActorPipeline ActorType = "pipeline"
	ActorGate     ActorType = "gate"
	ActorUser     ActorType = "user"
)

type EntityType string

const (
	EntityTask    EntityType = "task"
	EntityPlan    EntityType = "plan"
	EntitySession EntityType = "session"
)
```

**Acceptance criteria:**
- `ActorType` and `EntityType` are exported types with constants
- No existing code breaks -- these are additive types
- `go build ./...` succeeds

---

### TASK-007: Add actor and entity fields to ExecutionEvent

**Target files:**
- `internal/agent/middleware/events.go`

**Description:** Extend `ExecutionEvent` with `ActorType`, `ActorID`, `EntityType`, and `EntityID` fields. Bump `SchemaVersion` to 2 for events that include these fields. Events without actor fields default to `ActorPipeline`.

**Code example:**
```go
// internal/agent/middleware/events.go

type ExecutionEvent struct {
	SchemaVersion int            `json:"schema_version,omitempty"`
	Timestamp     string         `json:"timestamp"`
	Type          EventType      `json:"type"`
	// ... existing fields ...
	ActorType     ActorType      `json:"actor_type,omitempty"`
	ActorID       string         `json:"actor_id,omitempty"`
	EntityType    EntityType     `json:"entity_type,omitempty"`
	EntityID      string         `json:"entity_id,omitempty"`
}
```

**Acceptance criteria:**
- `ExecutionEvent` has `ActorType`, `ActorID`, `EntityType`, `EntityID` fields
- `SchemaVersion` defaults to 1 for backward compatibility
- Events emitted with actor fields use `SchemaVersion: 2`
- Existing event consumers (diagnose, eval) handle both v1 and v2 events
- `go test ./internal/agent/middleware/...` passes

---

### TASK-008: Emit actor-typed events from pipeline executor invocation

**Target files:**
- `internal/orchestration/pipeline/runner.go`

**Description:** When the pipeline invokes the executor, emit `agent_start` and `agent_complete` events with `ActorType: executor`, `ActorID: <agent-name>`, `EntityType: task`, `EntityID: <task-id>`.

**Code example:**
```go
// When emitting executor events:
event := middleware.ExecutionEvent{
	SchemaVersion: 2,
	Type:          middleware.EventAgentStart,
	ActorType:     middleware.ActorExecutor,
	ActorID:       string(executorAgent),
	EntityType:    middleware.EntityTask,
	EntityID:      task.ID,
	// ... other fields ...
}
```

**Acceptance criteria:**
- `agent_start` events from executor have `actor_type: "executor"` and `entity_type: "task"`
- `agent_complete` events from executor have `actor_type: "executor"` and `entity_type: "task"`
- `actor_id` is the agent name (e.g., "claude", "codex")
- `entity_id` is the task ID (e.g., "TASK-001")
- `go test ./internal/orchestration/pipeline/...` passes

---

### TASK-009: Emit actor-typed events from pipeline reviewer invocation

**Target files:**
- `internal/orchestration/pipeline/runner.go`

**Description:** When the pipeline invokes the reviewer, emit events with `ActorType: reviewer`. When gates run, emit events with `ActorType: gate`.

**Code example:**
```go
// Reviewer events:
event := middleware.ExecutionEvent{
	SchemaVersion: 2,
	Type:          middleware.EventAgentStart,
	ActorType:     middleware.ActorReviewer,
	ActorID:       string(reviewerAgent),
	EntityType:    middleware.EntityTask,
	EntityID:      task.ID,
}

// Gate events:
gateEvent := middleware.ExecutionEvent{
	SchemaVersion: 2,
	Type:          middleware.EventGateResult,
	ActorType:     middleware.ActorGate,
	ActorID:       gateName,       // e.g. "tests", "lint"
	EntityType:    middleware.EntityTask,
	EntityID:      task.ID,
}
```

**Acceptance criteria:**
- Reviewer `agent_start`/`agent_complete` events have `actor_type: "reviewer"`
- Gate `gate_result` events have `actor_type: "gate"` and `actor_id: <gate-name>`
- `go test ./internal/orchestration/pipeline/...` passes

---

### TASK-010: Emit actor-typed events for state transitions

**Target files:**
- `internal/orchestration/pipeline/runner.go`

**Description:** When task status changes (e.g., `pending -> executing`, `reviewing -> done`), emit `state_transition` events with `ActorType` reflecting who caused the transition (executor, reviewer, pipeline, gate).

**Code example:**
```go
// State transition event:
event := middleware.ExecutionEvent{
	SchemaVersion: 2,
	Type:          middleware.EventStateTransit,
	ActorType:     middleware.ActorPipeline,
	EntityType:    middleware.EntityTask,
	EntityID:      task.ID,
	Data: map[string]any{
		"from": string(oldStatus),
		"to":   string(newStatus),
	},
}
```

**Acceptance criteria:**
- `state_transition` events include `from` and `to` status values in `Data`
- Transitions caused by executor output use `actor_type: "executor"`
- Transitions caused by reviewer verdict use `actor_type: "reviewer"`
- Transitions caused by gate failure use `actor_type: "gate"`
- Transitions caused by pipeline logic (stall, retry exhaustion) use `actor_type: "pipeline"`
- `go test ./internal/orchestration/pipeline/...` passes

---

## Milestone 3: Structured Feedback

**Goal:** Implement session persistence so agents can resume conversations across retries, and accumulate per-agent statistics across plan runs.

### TASK-011: Define SessionState interface on Agent

**Target files:**
- `internal/agent/types.go`

**Description:** Add an optional `SessionState` interface that adapters can implement. Adapters that don't support sessions return `nil`. The pipeline persists whatever the adapter returns.

**Code example:**
```go
// internal/agent/types.go

// SessionState is optionally implemented by agents that support session persistence.
type SessionState interface {
	MarshalSession() ([]byte, error)
	UnmarshalSession(data []byte) error
	SessionID() string
}
```

**Acceptance criteria:**
- `SessionState` interface is defined and exported
- Existing `Agent` interface is NOT modified (no breaking change)
- `go build ./...` succeeds

---

### TASK-012: Implement SessionState for Claude adapter

**Target files:**
- `internal/agent/adapters/claude.go`

**Description:** The Claude adapter implements `SessionState` to persist `sessionId` and `cwd`. On execute, if a session is loaded, pass `--resume <sessionId>` to the Claude CLI.

**Code example:**
```go
// internal/agent/adapters/claude.go

type claudeSession struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

func (a *ClaudeCLI) MarshalSession() ([]byte, error) {
	if a.session == nil {
		return nil, nil
	}
	return json.Marshal(a.session)
}

func (a *ClaudeCLI) UnmarshalSession(data []byte) error {
	var s claudeSession
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	a.session = &s
	return nil
}

func (a *ClaudeCLI) SessionID() string {
	if a.session == nil {
		return ""
	}
	return a.session.SessionID
}
```

**Acceptance criteria:**
- Claude adapter implements `SessionState` interface
- `--resume <sessionId>` is passed when a session is loaded
- Session is populated from `--resume` output after execution
- If `--resume` fails (unknown session error), retry without `--resume` and clear session
- `go test ./internal/agent/adapters/...` passes

---

### TASK-013: Add session file storage to state store

**Target files:**
- `internal/state/store.go`
- `internal/state/store_session.go` (new file)

**Description:** Add methods to read/write session files at `state/<plan>/<task-id>/session.<agent>.json`. Use atomic writes (write to temp file, then `os.Rename`).

**Code example:**
```go
// internal/state/store_session.go

func (s *Store) WriteSession(slug, taskID, agentName string, data []byte) error {
	dir := filepath.Join(s.StateDir(), slug, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(dir, "session."+agentName+".json")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

func (s *Store) ReadSession(slug, taskID, agentName string) ([]byte, error) {
	path := filepath.Join(s.StateDir(), slug, taskID, "session."+agentName+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (s *Store) ClearSession(slug, taskID, agentName string) error {
	path := filepath.Join(s.StateDir(), slug, taskID, "session."+agentName+".json")
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
```

**Acceptance criteria:**
- `WriteSession` creates the directory tree and writes atomically via rename
- `ReadSession` returns `nil, nil` when no session file exists
- `ClearSession` removes the file without error if it doesn't exist
- Files are stored at `state/<slug>/<task-id>/session.<agent>.json`
- `go test ./internal/state/...` passes

---

### TASK-014: Integrate session persistence into pipeline

**Target files:**
- `internal/orchestration/pipeline/runner.go`

**Description:** Before executing a task, check if the agent implements `SessionState` and load any persisted session. After execution, persist the session. On task re-assignment or `--fresh` flag, clear the session.

**Code example:**
```go
// internal/orchestration/pipeline/runner.go - before executor invocation

if ss, ok := executorAgent.(agent.SessionState); ok {
	data, err := r.store.ReadSession(r.slug, task.ID, string(executorID))
	if err == nil && data != nil {
		_ = ss.UnmarshalSession(data)
	}
}

// After executor invocation:
if ss, ok := executorAgent.(agent.SessionState); ok {
	data, err := ss.MarshalSession()
	if err == nil && data != nil {
		_ = r.store.WriteSession(r.slug, task.ID, string(executorID), data)
	}
}
```

**Acceptance criteria:**
- Sessions are loaded before executor invocation for tasks with existing session files
- Sessions are saved after successful executor invocation
- Sessions are cleared when a task transitions back to `pending` (retry with different agent)
- Agents that don't implement `SessionState` are unaffected
- `go test ./internal/orchestration/pipeline/...` passes

---

### TASK-015: Implement graceful session retry on resume failure

**Target files:**
- `internal/agent/adapters/claude.go`

**Description:** In the Claude adapter's `Execute()` method, catch session-resume errors (exit code + stderr pattern matching for "unknown session"). Retry once without the session flag. If the retry succeeds, mark the session for clearing.

**Code example:**
```go
// internal/agent/adapters/claude.go

func (a *ClaudeCLI) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	resp, err := a.executeWithSession(ctx, req)
	if err != nil && a.session != nil && isUnknownSessionError(err, resp) {
		a.session = nil // clear stale session
		return a.executeWithSession(ctx, req) // retry fresh
	}
	return resp, err
}

func isUnknownSessionError(err error, resp ExecuteResponse) bool {
	return strings.Contains(err.Error(), "unknown session") ||
		strings.Contains(resp.Output, "unknown session")
}
```

**Acceptance criteria:**
- If `--resume` fails with "unknown session" error, adapter retries without `--resume`
- Session is cleared after a successful fresh retry
- Non-session errors are not retried
- `go test ./internal/agent/adapters/...` passes

---

### TASK-016: Add cumulative agent statistics file

**Target files:**
- `internal/state/store_stats.go` (new file)
- `internal/orchestration/pipeline/runner.go`

**Description:** After each task execution, append token/cost data to `stats/<agent-name>.json`. The file accumulates lifetime statistics per agent.

**Code example:**
```go
// internal/state/store_stats.go

type AgentStats struct {
	TotalCostCents float64 `json:"total_cost_cents"`
	TotalRuns      int     `json:"total_runs"`
	LastRunID      string  `json:"last_run_id"`
	LastError      string  `json:"last_error,omitempty"`
	UpdatedAt      string  `json:"updated_at"`
}

func (s *Store) UpdateAgentStats(agentName string, costCents float64, runID string, lastErr string) error {
	path := filepath.Join(s.Root, "stats", agentName+".json")
	var stats AgentStats
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &stats)
	}
	stats.TotalCostCents += costCents
	stats.TotalRuns++
	stats.LastRunID = runID
	stats.LastError = lastErr
	stats.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) ReadAgentStats(agentName string) (AgentStats, error) {
	path := filepath.Join(s.Root, "stats", agentName+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return AgentStats{}, nil
	}
	if err != nil {
		return AgentStats{}, err
	}
	var stats AgentStats
	return stats, json.Unmarshal(data, &stats)
}
```

**Acceptance criteria:**
- `stats/<agent-name>.json` file is created/updated after each task execution
- Statistics accumulate across multiple plan runs
- `total_cost_cents` is the sum of all cost values for that agent
- `total_runs` increments by 1 per invocation (execute + review each count)
- File uses atomic write (tmp + rename)
- `go test ./internal/state/...` passes

---

## Milestone 4: Plan Templates

**Goal:** Enable reusable plan templates with parameterized variables, reducing boilerplate for common workflows.

### TASK-017: Define plan template schema with variables

**Target files:**
- `internal/domain/types.go`

**Description:** Add a `Template` section to the plan schema that declares parameterized variables. Variables are substituted into task fields during plan loading.

**Code example:**
```go
// internal/domain/types.go

type PlanTemplate struct {
	Variables []TemplateVariable `json:"variables,omitempty"`
}

type TemplateVariable struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

type Plan struct {
	Name      string         `json:"name"`
	Summary   string         `json:"summary,omitempty"`
	Meta      PlanMeta       `json:"meta,omitempty"`
	Template  *PlanTemplate  `json:"template,omitempty"` // new
	Settings  PlanSettings   `json:"settings"`
	Quality   PlanQuality    `json:"quality,omitempty"`
	Cognitive *PlanCognitive `json:"cognitive,omitempty"`
	Tasks     []Task         `json:"tasks"`
}
```

**Acceptance criteria:**
- `PlanTemplate` and `TemplateVariable` types are defined
- `Plan` struct has optional `Template` field
- Existing plans without `template` continue to load unchanged
- `go test ./internal/domain/...` passes

---

### TASK-018: Implement variable substitution in plan loader

**Target files:**
- `internal/domain/plan.go`

**Description:** When loading a plan with a `template` section, substitute `{{.VarName}}` placeholders in task `title`, `description`, and `acceptance` fields with provided variable values.

**Code example:**
```go
// internal/domain/plan.go

func SubstituteTemplateVars(plan *Plan, vars map[string]string) error {
	if plan.Template == nil {
		return nil
	}
	// Validate required variables are provided
	for _, v := range plan.Template.Variables {
		if v.Required {
			if _, ok := vars[v.Name]; !ok {
				if v.Default == "" {
					return fmt.Errorf("required template variable %q not provided", v.Name)
				}
			}
		}
	}
	// Build substitution map with defaults
	resolved := make(map[string]string)
	for _, v := range plan.Template.Variables {
		resolved[v.Name] = v.Default
	}
	for k, v := range vars {
		resolved[k] = v
	}
	// Substitute in task fields
	for i := range plan.Tasks {
		plan.Tasks[i].Title = substituteVars(plan.Tasks[i].Title, resolved)
		plan.Tasks[i].Description = substituteVars(plan.Tasks[i].Description, resolved)
		for j := range plan.Tasks[i].Acceptance {
			plan.Tasks[i].Acceptance[j] = substituteVars(plan.Tasks[i].Acceptance[j], resolved)
		}
	}
	plan.Name = substituteVars(plan.Name, resolved)
	plan.Summary = substituteVars(plan.Summary, resolved)
	return nil
}

func substituteVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{."+k+"}}", v)
	}
	return s
}
```

**Acceptance criteria:**
- `{{.VarName}}` placeholders in task title, description, acceptance, plan name, and summary are replaced
- Required variables without defaults cause an error if not provided
- Default values are used when a variable is not explicitly provided
- Non-template plans are unaffected
- `go test ./internal/domain/...` passes

---

### TASK-019: Add --var flag to plan run and plan create

**Target files:**
- `internal/cli/plan_run.go`
- `internal/cli/plan_create.go`

**Description:** Add a `--var key=value` repeatable flag to `plan run` and `plan create` commands. Parse the flags into a `map[string]string` and pass to `SubstituteTemplateVars`.

**Code example:**
```go
// internal/cli/plan_run.go
var templateVars []string
cmd.Flags().StringArrayVar(&templateVars, "var", nil, "template variable (key=value), repeatable")

// Parse into map:
func parseTemplateVars(vars []string) (map[string]string, error) {
	m := make(map[string]string, len(vars))
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --var format %q, expected key=value", v)
		}
		m[parts[0]] = parts[1]
	}
	return m, nil
}
```

**Acceptance criteria:**
- `praetor plan run my-plan --var module=auth --var lang=go` passes variables to template substitution
- Invalid `--var` format (missing `=`) returns a clear error
- Flag is optional -- plans without templates work normally
- `go test ./internal/cli/...` passes

---

### TASK-020: Add plan template directory support

**Target files:**
- `internal/state/store.go`
- `internal/cli/plan_create.go`

**Description:** Support loading plan templates from a `templates/` directory under the project root. `plan create --from-template <name>` copies a template to `plans/` and applies variable substitution.

**Code example:**
```go
// internal/cli/plan_create.go

// --from-template flag loads from templates/ directory
cmd.Flags().StringVar(&fromTemplate, "from-template", "", "create plan from a named template in templates/")

// Load template:
func loadTemplate(store *state.Store, name string) (domain.Plan, error) {
	path := filepath.Join(store.Root, "templates", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Plan{}, fmt.Errorf("template %q not found: %w", name, err)
	}
	var plan domain.Plan
	return plan, json.Unmarshal(data, &plan)
}
```

**Acceptance criteria:**
- `praetor plan create --from-template auth-module --var module=payments` creates a plan from template
- Templates are stored in `<project-home>/templates/`
- Template variables are substituted before writing the plan
- Missing template returns a clear error message
- `go test ./internal/cli/...` passes

---

### TASK-021: Add plan list-templates command

**Target files:**
- `internal/cli/plan.go`
- `internal/cli/plan_list_templates.go` (new file)

**Description:** Add a `praetor plan list-templates` subcommand that lists available templates with their declared variables.

**Code example:**
```go
// internal/cli/plan_list_templates.go

func newPlanListTemplatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-templates",
		Short: "List available plan templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Scan templates/ directory
			// For each .json file, parse and display name + variables
			return nil
		},
	}
}
```

**Acceptance criteria:**
- `praetor plan list-templates` lists all `.json` files in `templates/`
- Output shows template name, summary, and declared variables with defaults
- Empty `templates/` directory shows "no templates found"
- `go build ./...` succeeds

---

## Milestone 5: Richer Health Checks

**Goal:** Inject structured environment variables into agent subprocesses and enrich agent probing with model-level health checks.

### TASK-022: Define PRAETOR_* environment variable set

**Target files:**
- `internal/agent/runner/command.go`

**Description:** Define a set of `PRAETOR_*` environment variables injected into every agent subprocess. These provide structured context that agents can read.

**Code example:**
```go
// internal/agent/runner/command.go

func praetorEnvVars(planSlug, taskID, runID, phase string, attempt int, projectRoot string) []string {
	return []string{
		"PRAETOR_PLAN=" + planSlug,
		"PRAETOR_TASK_ID=" + taskID,
		"PRAETOR_RUN_ID=" + runID,
		"PRAETOR_PHASE=" + phase,
		"PRAETOR_ATTEMPT=" + strconv.Itoa(attempt),
		"PRAETOR_PROJECT_ROOT=" + projectRoot,
	}
}
```

**Acceptance criteria:**
- Function returns a slice of `KEY=VALUE` strings
- All 6 variables are defined: `PRAETOR_PLAN`, `PRAETOR_TASK_ID`, `PRAETOR_RUN_ID`, `PRAETOR_PHASE`, `PRAETOR_ATTEMPT`, `PRAETOR_PROJECT_ROOT`
- `go build ./...` succeeds

---

### TASK-023: Inject PRAETOR_* env vars into CommandSpec

**Target files:**
- `internal/agent/runner/command.go`
- `internal/orchestration/pipeline/runner.go`

**Description:** When the pipeline builds a `CommandSpec` for agent invocation, append the `PRAETOR_*` environment variables to `spec.Env`. Pass plan/task/run metadata through the request chain.

**Code example:**
```go
// internal/agent/runner/command.go - in Run():
func (r *execCommandRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	// Existing env vars are already in spec.Env
	// PRAETOR_* vars are appended by the pipeline before calling Run()
	// ...
}
```

**Acceptance criteria:**
- Agent subprocesses receive `PRAETOR_*` environment variables
- Variables are set correctly for both executor and reviewer phases
- `PRAETOR_PHASE` is "execute" or "review" depending on phase
- `PRAETOR_ATTEMPT` reflects the current retry count
- Existing environment variables are preserved (appended, not replaced)
- `go test ./internal/orchestration/pipeline/...` passes

---

### TASK-024: Add model field to agent probe results

**Target files:**
- `internal/agent/prober.go`

**Description:** Extend the probe result to include which models the agent supports (where detectable). For CLI agents that support `--list-models` or equivalent, capture available models.

**Code example:**
```go
// internal/agent/prober.go

type ProbeResult struct {
	ID        ID
	Available bool
	Version   string
	Models    []string // new: detected available models
	Error     string
}
```

**Acceptance criteria:**
- `ProbeResult` includes a `Models` field
- Probe results populate `Models` where the agent CLI supports model listing
- Agents that don't support model listing return an empty slice
- Existing probe behavior is unchanged
- `go test ./internal/agent/...` passes

---

### TASK-025: Display model availability in praetor doctor

**Target files:**
- `internal/cli/doctor.go`

**Description:** Enhance `praetor doctor` output to show detected models for each available agent, using the enriched `ProbeResult`.

**Code example:**
```go
// internal/cli/doctor.go - in doctor output:
// claude    ok     v1.2.3   models: opus, sonnet
// codex     ok     v2.0.0   models: gpt-5-codex
// ollama    ok     v0.8.0   models: llama3, mistral
```

**Acceptance criteria:**
- `praetor doctor` shows available models for each healthy agent
- Agents without model detection show "models: -"
- Output format is consistent with existing doctor table
- `go build ./...` succeeds

---

## Milestone 6: Parallel Execution

**Goal:** Enable concurrent execution of independent tasks in a plan's dependency DAG, with file-based task locks to prevent double-execution.

### TASK-026: Add file-based task lock mechanism

**Target files:**
- `internal/state/store_task_lock.go` (new file)

**Description:** Implement per-task lock files at `locks/<plan>/<task-id>.lock` using `O_CREATE|O_EXCL` for atomic creation. Include PID-based stale lock detection.

**Code example:**
```go
// internal/state/store_task_lock.go

type TaskLock struct {
	RunID     string `json:"run_id"`
	ClaimedAt string `json:"claimed_at"`
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
}

func (s *Store) AcquireTaskLock(slug, taskID, runID string) error {
	dir := filepath.Join(s.LocksDir(), slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, taskID+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			if s.isTaskLockStale(path) {
				_ = os.Remove(path)
				return s.AcquireTaskLock(slug, taskID, runID) // retry once
			}
			return fmt.Errorf("task %s is locked by another process", taskID)
		}
		return err
	}
	defer f.Close()
	lock := TaskLock{
		RunID:     runID,
		ClaimedAt: time.Now().UTC().Format(time.RFC3339),
		PID:       os.Getpid(),
		Hostname:  hostname(),
	}
	return json.NewEncoder(f).Encode(lock)
}

func (s *Store) ReleaseTaskLock(slug, taskID string) error {
	path := filepath.Join(s.LocksDir(), slug, taskID+".lock")
	return os.Remove(path)
}

func (s *Store) isTaskLockStale(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var lock TaskLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return true
	}
	proc, err := os.FindProcess(lock.PID)
	if err != nil {
		return true
	}
	return proc.Signal(syscall.Signal(0)) != nil
}
```

**Acceptance criteria:**
- `AcquireTaskLock` creates a lock file atomically with `O_CREATE|O_EXCL`
- `AcquireTaskLock` fails if lock exists and is not stale
- Stale lock detection checks PID liveness via `Signal(0)`
- `ReleaseTaskLock` removes the lock file
- Lock file contains JSON with `run_id`, `claimed_at`, `pid`, `hostname`
- `go test ./internal/state/...` passes

---

### TASK-027: Extend DAG walker to return all runnable tasks

**Target files:**
- `internal/domain/transition.go`

**Description:** Modify `NextRunnableTask` (or add `NextRunnableTasks`) to return all tasks whose dependencies are satisfied and status is `pending`, not just the first one.

**Code example:**
```go
// internal/domain/transition.go

func NextRunnableTasks(plan Plan, state State) []StateTask {
	done := make(map[string]bool)
	for _, t := range state.Tasks {
		if t.Status == TaskDone {
			done[t.ID] = true
		}
	}
	var runnable []StateTask
	for _, t := range state.Tasks {
		if t.Status != TaskPending {
			continue
		}
		task := findPlanTask(plan, t.ID)
		if task == nil {
			continue
		}
		allDepsMet := true
		for _, dep := range task.DependsOn {
			if !done[dep] {
				allDepsMet = false
				break
			}
		}
		if allDepsMet {
			runnable = append(runnable, t)
		}
	}
	return runnable
}
```

**Acceptance criteria:**
- `NextRunnableTasks` returns all tasks with satisfied dependencies and `pending` status
- Returns empty slice when no tasks are runnable
- Existing `NextRunnableTask` (singular) continues to work unchanged
- `go test ./internal/domain/...` passes

---

### TASK-028: Add --parallel flag and parallel execution mode

**Target files:**
- `internal/cli/plan_run.go`
- `internal/domain/types.go` (RunnerOptions)

**Description:** Add a `--parallel` flag (and `--parallel-workers N`) to `plan run`. When enabled, the pipeline uses concurrent execution for independent tasks.

**Code example:**
```go
// internal/domain/types.go
type RunnerOptions struct {
	// ... existing fields ...
	Parallel        bool
	ParallelWorkers int
}

// internal/cli/plan_run.go
cmd.Flags().BoolVar(&opts.Parallel, "parallel", false, "execute independent tasks concurrently")
cmd.Flags().IntVar(&opts.ParallelWorkers, "parallel-workers", 2, "max concurrent task executions")
```

**Acceptance criteria:**
- `--parallel` flag is available on `plan run`
- `--parallel-workers` defaults to 2
- Workers value is clamped to 1-8 range
- Flag values are available in `RunnerOptions`
- `go test ./internal/cli/...` passes

---

### TASK-029: Implement parallel task dispatcher in pipeline

**Target files:**
- `internal/orchestration/pipeline/runner_parallel.go` (new file)

**Description:** When `--parallel` is enabled, the FSM's task selection phase uses `NextRunnableTasks` to find all runnable tasks, acquires task locks, and dispatches up to `ParallelWorkers` concurrent goroutines. Each goroutine runs the standard execute-review cycle.

**Code example:**
```go
// internal/orchestration/pipeline/runner_parallel.go

func (r *activeRun) runParallelIteration(ctx context.Context) error {
	runnable := domain.NextRunnableTasks(r.plan, r.state)
	if len(runnable) == 0 {
		return nil
	}
	workers := min(len(runnable), r.opts.ParallelWorkers)

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	errs := make([]error, len(runnable))

	for i, task := range runnable[:workers] {
		if err := r.store.AcquireTaskLock(r.slug, task.ID, r.runID); err != nil {
			continue // skip locked tasks
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, t domain.StateTask) {
			defer wg.Done()
			defer func() { <-sem }()
			defer r.store.ReleaseTaskLock(r.slug, t.ID)
			errs[idx] = r.executeAndReviewTask(ctx, t)
		}(i, task)
	}
	wg.Wait()
	return errors.Join(errs...)
}
```

**Acceptance criteria:**
- Independent tasks execute concurrently up to `ParallelWorkers` limit
- Each task acquires a lock before execution and releases it after
- Tasks with unsatisfied dependencies are not dispatched
- State mutations are synchronized (snapshots are written atomically)
- Errors from individual tasks don't crash the pipeline
- `go test ./internal/orchestration/pipeline/...` passes

---

### TASK-030: Ensure worktree isolation for parallel tasks

**Target files:**
- `internal/orchestration/pipeline/runner_parallel.go`
- `internal/workspace/workspace.go`

**Description:** When running tasks in parallel, each task must use a separate git worktree. Verify that the existing worktree isolation mechanism supports concurrent creation and cleanup.

**Code example:**
```go
// Each parallel task gets its own worktree:
// worktrees/<plan>-<task-id>/

func (r *activeRun) prepareParallelWorktree(taskID string) (string, func(), error) {
	worktreePath := filepath.Join(r.opts.Workdir, ".praetor", "worktrees", r.slug+"-"+taskID)
	// Create worktree via git worktree add
	// Return cleanup function that removes worktree
	return worktreePath, cleanup, nil
}
```

**Acceptance criteria:**
- Each parallel task executes in a separate git worktree
- Worktree paths are unique per task: `worktrees/<plan>-<task-id>/`
- Worktrees are cleaned up after task completion
- Concurrent worktree creation does not cause git lock conflicts
- If worktree creation fails, the task is skipped (not crashed)
- `go test ./internal/orchestration/pipeline/...` passes

---

## Appendix A: Discarded Concepts with Reasoning

### A.1 Corporate Hierarchy (CEO, CTO, Engineer roles)

**Source:** Paperclip's 11-role agent hierarchy with `reportsTo` tree.

**Why discarded:** Praetor orchestrates tasks, not organizations. The 11-role hierarchy assumes agents are persistent organizational members. Praetor treats agents as interchangeable execution backends -- the same task could run on Claude, Codex, or Gemini. Adding CEO/CTO roles would introduce complexity without benefit for plan-driven execution.

**What we keep instead:** Praetor's functional roles (executor, reviewer, planner) are sufficient and provider-agnostic.

### A.2 Human Board Approval Gates

**Source:** Paperclip's `hire_agent` and `approve_ceo_strategy` approval types.

**Why discarded:** Praetor is a CLI tool designed for automated, batch execution. Adding board approval gates would require a running server, persistent human sessions, and would break the run-and-done execution model. Praetor's automated quality gates (tests, lint, standards) and reviewer agent provide equivalent safety without human intervention.

### A.3 PostgreSQL Database

**Source:** Paperclip's 35-table Drizzle ORM schema.

**Why discarded:** Praetor's filesystem-based state (JSON, JSONL, TSV) is deliberately zero-dependency. Adding a database would contradict the "single binary, no infrastructure" design principle. The operational overhead of migrations, connection management, and deployment is not justified for a CLI tool.

### A.4 React SPA and WebSocket UI

**Source:** Paperclip's dashboard, Kanban board, org chart, cost analytics views.

**Why discarded:** Building a web UI would be a massive scope expansion. Praetor's CLI-first philosophy serves developers who live in terminals. MCP integration provides programmatic access for AI agents. A future TUI could be built with terminal libraries if visual monitoring is needed.

### A.5 Multi-Tenancy (Company-Level Isolation)

**Source:** Paperclip's `companyId` scoping on every table.

**Why discarded:** Praetor runs as a local CLI tool under the user's permissions. Multi-tenant isolation has no parallel -- each project is already isolated by filesystem path.

### A.6 Heartbeat-Based Scheduling

**Source:** Paperclip's timer-based wakeups with interval policies and max concurrent runs.

**Why discarded:** Praetor's execution is plan-driven -- a run starts, tasks execute, the run ends. There is no idle period to wake agents from. If continuous operation is needed, it should be driven by an external scheduler (cron, systemd timer) invoking `praetor plan run`.

### A.7 Issue-Based Inter-Agent Communication

**Source:** Paperclip's issue comments, @mentions, and wakeup-on-mention triggers.

**Why discarded:** Praetor's agents don't communicate with each other. The pipeline mediates all information flow: executor output feeds reviewer input, reviewer feedback feeds retry context. Adding agent-to-agent messaging would require persistent state, a running server, and would violate the pipeline's unidirectional data flow.

### A.8 Wakeup Coalescing (Full Implementation)

**Source:** Paperclip's 6-outcome wakeup coalescing logic.

**Why discarded:** The full coalescing system (including deferred wakeups, coalescence counting, and context merging) is designed for an always-on system with concurrent wakeup sources. Praetor has a single execution trigger: `plan run`. The core principle (deduplicate redundant work) is preserved through simpler mechanisms -- task locks (TASK-026) prevent double-execution, and session persistence (TASK-014) provides context continuity.

### A.9 Agent Hiring and Onboarding Workflow

**Source:** Paperclip's approval-gated agent creation with connection strings.

**Why discarded:** Praetor agents are configured in TOML, probed at startup, and used during execution. No "hiring" is needed. `praetor doctor` already validates agent health.

### A.10 Skill Injection via Tmpdir Symlinks

**Source:** Paperclip's Claude adapter tmpdir skill injection pattern.

**Why discarded:** Praetor's prompt template system and shared agent commands (`.agents/commands/`) already handle context injection. The tmpdir symlink pattern adds complexity for marginal benefit. Per-task dynamic context is better handled through prompt templates and the `PRAETOR_*` environment variables introduced in M5.

---

## Appendix B: Summary Table

| Task | Milestone | Target Files | Effort | Dependencies |
|------|-----------|-------------|--------|--------------|
| TASK-001 | M1 | `domain/types.go` | Low | None |
| TASK-002 | M1 | `cli/plan_run.go`, `config/config.go`, `domain/types.go` | Low | TASK-001 |
| TASK-003 | M1 | `domain/types.go`, `state/store_metrics.go`, `middleware/events.go` | Low | None |
| TASK-004 | M1 | `pipeline/runner.go` | Low | TASK-001, TASK-003 |
| TASK-005 | M1 | `pipeline/runner.go` | Low | TASK-004 |
| TASK-006 | M2 | `middleware/events.go` | Low | None |
| TASK-007 | M2 | `middleware/events.go` | Low | TASK-006 |
| TASK-008 | M2 | `pipeline/runner.go` | Low | TASK-007 |
| TASK-009 | M2 | `pipeline/runner.go` | Low | TASK-007 |
| TASK-010 | M2 | `pipeline/runner.go` | Medium | TASK-008, TASK-009 |
| TASK-011 | M3 | `agent/types.go` | Low | None |
| TASK-012 | M3 | `agent/adapters/claude.go` | Medium | TASK-011 |
| TASK-013 | M3 | `state/store.go`, `state/store_session.go` | Low | None |
| TASK-014 | M3 | `pipeline/runner.go` | Medium | TASK-011, TASK-013 |
| TASK-015 | M3 | `agent/adapters/claude.go` | Low | TASK-012 |
| TASK-016 | M3 | `state/store_stats.go`, `pipeline/runner.go` | Low | TASK-003 |
| TASK-017 | M4 | `domain/types.go` | Low | None |
| TASK-018 | M4 | `domain/plan.go` | Low | TASK-017 |
| TASK-019 | M4 | `cli/plan_run.go`, `cli/plan_create.go` | Low | TASK-018 |
| TASK-020 | M4 | `state/store.go`, `cli/plan_create.go` | Low | TASK-018 |
| TASK-021 | M4 | `cli/plan.go`, `cli/plan_list_templates.go` | Low | TASK-020 |
| TASK-022 | M5 | `agent/runner/command.go` | Low | None |
| TASK-023 | M5 | `agent/runner/command.go`, `pipeline/runner.go` | Low | TASK-022 |
| TASK-024 | M5 | `agent/prober.go` | Low | None |
| TASK-025 | M5 | `cli/doctor.go` | Low | TASK-024 |
| TASK-026 | M6 | `state/store_task_lock.go` | Low | None |
| TASK-027 | M6 | `domain/transition.go` | Low | None |
| TASK-028 | M6 | `cli/plan_run.go`, `domain/types.go` | Low | None |
| TASK-029 | M6 | `pipeline/runner_parallel.go` | High | TASK-026, TASK-027, TASK-028 |
| TASK-030 | M6 | `pipeline/runner_parallel.go`, `workspace/workspace.go` | Medium | TASK-029 |
