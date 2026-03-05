package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

func TestEvaluatePlanFlowDetectsQualityFailures(t *testing.T) {
	t.Parallel()

	store := localstate.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	plan := domain.Plan{
		Name: "quality-plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Quality: domain.PlanQuality{
			Required: []string{"tests", "lint"},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "ok", Acceptance: []string{"done"}},
			{ID: "TASK-002", Title: "bad", Acceptance: []string{"done"}},
		},
	}
	if err := domain.WriteJSONFile(store.PlanFile("quality-plan"), plan); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	state := domain.State{
		PlanSlug:     "quality-plan",
		PlanChecksum: "checksum-1",
		Tasks: []domain.StateTask{
			{ID: "TASK-001", Title: "ok", Status: domain.TaskDone, Attempt: 0},
			{ID: "TASK-002", Title: "bad", Status: domain.TaskFailed, Attempt: 2},
		},
		Outcome: domain.RunFailed,
	}
	events := []map[string]any{
		{
			"timestamp":  "2026-03-05T10:00:00Z",
			"event_type": "gate_result",
			"task_id":    "TASK-001",
			"data": map[string]any{
				"task_id":  "TASK-001",
				"gate":     "tests",
				"status":   "PASS",
				"required": true,
			},
		},
		{
			"timestamp":  "2026-03-05T10:00:01Z",
			"event_type": "gate_result",
			"task_id":    "TASK-001",
			"data": map[string]any{
				"task_id":  "TASK-001",
				"gate":     "lint",
				"status":   "PASS",
				"required": true,
			},
		},
		{
			"timestamp":  "2026-03-05T10:00:10Z",
			"event_type": "agent_complete",
			"task_id":    "TASK-001",
			"cost_usd":   0.12,
		},
		{
			"timestamp":  "2026-03-05T10:01:00Z",
			"event_type": "gate_result",
			"task_id":    "TASK-002",
			"data": map[string]any{
				"task_id":  "TASK-002",
				"gate":     "tests",
				"status":   "FAIL",
				"required": true,
			},
		},
		{
			"timestamp":  "2026-03-05T10:01:10Z",
			"event_type": "agent_error",
			"task_id":    "TASK-002",
			"cost_usd":   0.03,
			"data": map[string]any{
				"task_id":           "TASK-002",
				"parse_error_class": "non_recoverable",
			},
		},
		{
			"timestamp":  "2026-03-05T10:01:20Z",
			"event_type": "task_stalled",
			"task_id":    "TASK-002",
		},
	}
	writeFlowRunArtifacts(t, store, "run-100", state, events, "2026-03-05T10:01:20Z")

	report, err := EvaluatePlanFlow(store, "quality-plan", "run-100")
	if err != nil {
		t.Fatalf("evaluate plan flow: %v", err)
	}

	if report.Summary.Verdict != evalVerdictFail {
		t.Fatalf("verdict=%q, want %q", report.Summary.Verdict, evalVerdictFail)
	}
	if report.Summary.AcceptedCount != 1 {
		t.Fatalf("accepted_count=%d, want 1", report.Summary.AcceptedCount)
	}
	if report.Summary.RequiredGateFailureTasks != 1 {
		t.Fatalf("required_gate_failure_tasks=%d, want 1", report.Summary.RequiredGateFailureTasks)
	}
	if report.Summary.RequiredGateMissingTasks != 1 {
		t.Fatalf("required_gate_missing_tasks=%d, want 1", report.Summary.RequiredGateMissingTasks)
	}
	if report.Summary.ParseErrorTasks != 1 {
		t.Fatalf("parse_error_tasks=%d, want 1", report.Summary.ParseErrorTasks)
	}

	taskTwo := findFlowTask(report.Tasks, "TASK-002")
	if taskTwo.TaskID == "" {
		t.Fatal("expected TASK-002 in report")
	}
	if taskTwo.Accepted {
		t.Fatal("TASK-002 should not be accepted")
	}
	if taskTwo.RequiredGatePass {
		t.Fatal("TASK-002 should fail required gate checks")
	}
}

func TestEvaluateProjectFlowAggregatesPlanVerdicts(t *testing.T) {
	t.Parallel()

	store := localstate.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	passPlan := domain.Plan{
		Name: "pass-plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "ok", Acceptance: []string{"done"}},
		},
	}
	if err := domain.WriteJSONFile(store.PlanFile("pass-plan"), passPlan); err != nil {
		t.Fatalf("write pass plan: %v", err)
	}

	warnPlan := domain.Plan{
		Name: "warn-plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "warn", Acceptance: []string{"done"}},
		},
	}
	if err := domain.WriteJSONFile(store.PlanFile("warn-plan"), warnPlan); err != nil {
		t.Fatalf("write warn plan: %v", err)
	}

	writeFlowRunArtifacts(t, store, "run-pass", domain.State{
		PlanSlug:     "pass-plan",
		PlanChecksum: "checksum-pass",
		Tasks: []domain.StateTask{
			{ID: "TASK-001", Title: "ok", Status: domain.TaskDone, Attempt: 0},
		},
		Outcome: domain.RunSuccess,
	}, []map[string]any{
		{"timestamp": "2026-03-05T12:00:00Z", "event_type": "agent_complete", "task_id": "TASK-001", "cost_usd": 0.05},
	}, "2026-03-05T12:00:00Z")

	writeFlowRunArtifacts(t, store, "run-warn", domain.State{
		PlanSlug:     "warn-plan",
		PlanChecksum: "checksum-warn",
		Tasks: []domain.StateTask{
			{ID: "TASK-001", Title: "warn", Status: domain.TaskDone, Attempt: 1},
		},
		Outcome: domain.RunSuccess,
	}, []map[string]any{
		{"timestamp": "2026-03-05T12:01:00Z", "event_type": "task_stalled", "task_id": "TASK-001"},
		{"timestamp": "2026-03-05T12:01:05Z", "event_type": "agent_complete", "task_id": "TASK-001", "cost_usd": 0.08},
	}, "2026-03-05T12:01:05Z")

	report, err := EvaluateProjectFlow(store, 0)
	if err != nil {
		t.Fatalf("evaluate project flow: %v", err)
	}

	if report.Summary.PlanCount != 2 {
		t.Fatalf("plan_count=%d, want 2", report.Summary.PlanCount)
	}
	if report.Summary.PassCount != 1 {
		t.Fatalf("pass_count=%d, want 1", report.Summary.PassCount)
	}
	if report.Summary.WarnCount != 1 {
		t.Fatalf("warn_count=%d, want 1", report.Summary.WarnCount)
	}
	if report.Summary.Verdict != evalVerdictWarn {
		t.Fatalf("project verdict=%q, want %q", report.Summary.Verdict, evalVerdictWarn)
	}
}

func writeFlowRunArtifacts(t *testing.T, store *localstate.Store, runID string, state domain.State, events []map[string]any, timestamp string) {
	t.Helper()

	runDir := filepath.Join(store.RuntimeDir(), runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	statePayload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	snapshot := localstate.Snapshot{
		Version:      1,
		RunID:        runID,
		PlanSlug:     state.PlanSlug,
		PlanChecksum: state.PlanChecksum,
		Outcome:      strings.TrimSpace(string(state.Outcome)),
		Timestamp:    strings.TrimSpace(timestamp),
		State:        statePayload,
	}
	snapshotData, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "snapshot.json"), snapshotData, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	var builder strings.Builder
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		builder.Write(line)
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}
}

func findFlowTask(tasks []PlanTaskEvalResult, id string) PlanTaskEvalResult {
	for _, item := range tasks {
		if item.TaskID == id {
			return item
		}
	}
	return PlanTaskEvalResult{}
}
