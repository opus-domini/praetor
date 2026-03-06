package mcp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/state"
)

func TestPlanDiagnoseReturnsActorAnalysisAndBudget(t *testing.T) {
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitMCP(t, repo, "init")

	projectHome, err := state.ResolveProjectHome("", repo)
	if err != nil {
		t.Fatalf("resolve project home: %v", err)
	}
	store := state.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	slug := "demo-plan"
	enforce := true
	plan := domain.Plan{
		Name: "Demo Plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
			ExecutionPolicy: domain.ExecutionPolicy{
				Cost: domain.CostPolicy{
					PlanLimitCents: 500,
					TaskLimitCents: 200,
					Enforce:        &enforce,
				},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "Task", Acceptance: []string{"done"}},
		},
	}
	writePlanJSONMCP(t, store.PlanFile(slug), plan)

	checksum, err := domain.PlanChecksum(store.PlanFile(slug))
	if err != nil {
		t.Fatalf("plan checksum: %v", err)
	}
	runSummary := domain.RunSummary{
		TotalCostUSD: 1.5,
		TasksDone:    1,
		ByActor: map[string]domain.ActorStats{
			"executor:codex":  {CostUSD: 1.5, Calls: 1, Retries: 2, Stalls: 1},
			"reviewer:claude": {CostUSD: 0.2, Calls: 1},
		},
	}
	stateValue := domain.State{
		PlanSlug:        slug,
		PlanChecksum:    checksum,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		ExecutionPolicy: plan.Settings.ExecutionPolicy,
		TotalCostMicros: 1_500_000,
		Outcome:         domain.RunSuccess,
		Tasks:           domain.StateTasksFromPlan(plan),
	}
	stateValue.Tasks[0].Status = domain.TaskDone
	if err := store.WriteState(slug, stateValue); err != nil {
		t.Fatalf("write state: %v", err)
	}

	snapshotStore := state.NewLocalSnapshotStore(store.RuntimeDir(), "run-1")
	if err := snapshotStore.Init(slug, checksum); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := snapshotStore.Save(state.LocalSnapshot{
		RunID:        "run-1",
		PlanSlug:     slug,
		PlanChecksum: checksum,
		ProjectRoot:  repo,
		Phase:        "finalize",
		Message:      "completed",
		Outcome:      domain.RunSuccess,
		Iteration:    1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Summary:      runSummary,
		State:        stateValue,
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	runDir := filepath.Join(store.RuntimeDir(), "run-1")
	if err := os.MkdirAll(filepath.Join(runDir, "diagnostics"), 0o755); err != nil {
		t.Fatalf("create diagnostics dir: %v", err)
	}
	writeJSONLMCP(t, filepath.Join(runDir, "events.jsonl"), []map[string]any{
		{"event_type": "task_started", "task_id": "TASK-001", "actor": map[string]any{"role": "executor", "agent": "codex"}},
		{"event_type": "cost_budget_warning", "cost_usd": 1.5, "actor": map[string]any{"role": "executor", "agent": "codex"}},
	})
	writeJSONLMCP(t, filepath.Join(runDir, "diagnostics", "performance.jsonl"), []map[string]any{
		{"phase": "execute", "prompt_chars": 1200, "estimated_tokens": 300},
	})

	server := NewServer(repo)
	blocks, err := server.tools.call("plan_diagnose", map[string]any{"slug": slug, "query": "all"})
	if err != nil {
		t.Fatalf("call plan_diagnose: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(blocks))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(blocks[0].Text), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	summary, ok := payload["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing or invalid: %#v", payload["summary"])
	}
	if got := summary["plan_limit_usd"]; got != float64(5) {
		t.Fatalf("plan_limit_usd = %v, want 5", got)
	}
	actorAnalysis, ok := payload["actor_analysis"].(map[string]any)
	if !ok {
		t.Fatalf("actor_analysis missing or invalid: %#v", payload["actor_analysis"])
	}
	if got := actorAnalysis["most_retries_actor"]; got != "executor:codex" {
		t.Fatalf("most_retries_actor = %v, want executor:codex", got)
	}
	costByActor, ok := actorAnalysis["cost_by_actor"].(map[string]any)
	if !ok {
		t.Fatalf("cost_by_actor missing or invalid: %#v", actorAnalysis["cost_by_actor"])
	}
	if got := costByActor["executor:codex"]; got != float64(1.5) {
		t.Fatalf("executor cost = %v, want 1.5", got)
	}
}

func TestPlanEventsReadsEventsJSONL(t *testing.T) {
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitMCP(t, repo, "init")

	projectHome, err := state.ResolveProjectHome("", repo)
	if err != nil {
		t.Fatalf("resolve project home: %v", err)
	}
	store := state.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	slug := "events-plan"
	plan := domain.Plan{
		Name: "Events Plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "Task", Acceptance: []string{"done"}},
		},
	}
	writePlanJSONMCP(t, store.PlanFile(slug), plan)
	checksum, err := domain.PlanChecksum(store.PlanFile(slug))
	if err != nil {
		t.Fatalf("plan checksum: %v", err)
	}
	snapshotStore := state.NewLocalSnapshotStore(store.RuntimeDir(), "run-2")
	if err := snapshotStore.Init(slug, checksum); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := snapshotStore.Save(state.LocalSnapshot{
		RunID:        "run-2",
		PlanSlug:     slug,
		PlanChecksum: checksum,
		ProjectRoot:  repo,
		Phase:        "loop",
		Message:      "running",
		Iteration:    1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		State: domain.State{
			PlanSlug:     slug,
			PlanChecksum: checksum,
			Tasks:        domain.StateTasksFromPlan(plan),
		},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	writeJSONLMCP(t, filepath.Join(store.RuntimeDir(), "run-2", "events.jsonl"), []map[string]any{
		{"event_type": "task_started", "task_id": "TASK-001"},
		{"event_type": "task_completed", "task_id": "TASK-001"},
	})

	server := NewServer(repo)
	blocks, err := server.tools.call("plan_events", map[string]any{"slug": slug, "last_n": 1})
	if err != nil {
		t.Fatalf("call plan_events: %v", err)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(blocks[0].Text), &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if got := events[0]["event_type"]; got != "task_completed" {
		t.Fatalf("event_type = %v, want task_completed", got)
	}
}

func writePlanJSONMCP(t *testing.T, path string, plan domain.Plan) {
	t.Helper()
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create plan dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

func writeJSONLMCP(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	var builder strings.Builder
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create jsonl dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
}

func gitMCP(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}
