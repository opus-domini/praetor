package loop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrInitializeStateMergesPlanChanges(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan.json")

	plan := Plan{
		Title: "sample",
		Tasks: []Task{
			{ID: "TASK-001", Title: "First task"},
			{ID: "TASK-002", Title: "Second task", DependsOn: []string{"TASK-001"}},
		},
	}
	writePlanFile(t, planPath, plan)

	store := NewStore(filepath.Join(tmpDir, "state"))
	state, err := store.LoadOrInitializeState(planPath, plan)
	if err != nil {
		t.Fatalf("initialize state: %v", err)
	}

	state.Tasks[0].Status = TaskStatusDone
	if err := store.WriteState(planPath, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	plan.Tasks = append(plan.Tasks, Task{ID: "TASK-003", Title: "Third task", DependsOn: []string{"TASK-002"}})
	writePlanFile(t, planPath, plan)

	merged, err := store.LoadOrInitializeState(planPath, plan)
	if err != nil {
		t.Fatalf("merge state: %v", err)
	}

	if len(merged.Tasks) != 3 {
		t.Fatalf("unexpected merged task count: %d", len(merged.Tasks))
	}
	if merged.Tasks[0].Status != TaskStatusDone {
		t.Fatalf("expected first task to remain done")
	}
	if merged.Tasks[2].Status != TaskStatusOpen {
		t.Fatalf("expected new task to be open")
	}
}

func writePlanFile(t *testing.T, path string, plan Plan) {
	t.Helper()
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("encode plan: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

func TestSaveAndDiscardGitSnapshot(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := &Store{Root: tmpDir}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	runID := "test-run-001"
	snapshotPath := filepath.Join(store.SnapshotsDir(), runID+".sha")

	// Write a fake snapshot
	if err := os.WriteFile(snapshotPath, []byte("abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := store.DiscardGitSnapshot(runID); err != nil {
		t.Fatalf("discard failed: %v", err)
	}

	if _, err := os.Stat(snapshotPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("snapshot file should be removed after discard")
	}
}

func TestDiscardGitSnapshotMissing(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := &Store{Root: tmpDir}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	if err := store.DiscardGitSnapshot("nonexistent"); err != nil {
		t.Fatalf("discard of missing snapshot should not error: %v", err)
	}
}

func TestWriteTaskMetrics(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := &Store{Root: tmpDir}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	entry := CostEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		RunID:     "run-001",
		TaskID:    "TASK-001",
		Agent:     "codex",
		Role:      "executor",
		DurationS: 12.5,
		Status:    "pass",
		CostUSD:   0.05,
	}
	if err := store.WriteTaskMetrics(entry); err != nil {
		t.Fatalf("write metrics: %v", err)
	}

	path := filepath.Join(store.CostsDir(), "tracking.tsv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tracking file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "timestamp\t") {
		t.Error("expected header row")
	}
	if !strings.Contains(content, "TASK-001") {
		t.Error("expected task ID in tracking")
	}
}

func TestWriteCheckpoint(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	store := &Store{Root: tmpDir}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	entry := CheckpointEntry{
		Timestamp: "2026-01-01T00:00:00Z",
		Status:    "completed",
		TaskID:    "TASK-001",
		Signature: "abc123",
		RunID:     "run-001",
		Message:   "task completed",
	}
	if err := store.WriteCheckpoint("test-plan.json", entry); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	historyPath := filepath.Join(store.CheckpointsDir(), "history.tsv")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if !strings.Contains(string(data), "TASK-001") {
		t.Error("expected task ID in history")
	}

	currentPath := filepath.Join(store.CheckpointsDir(), "test-plan.state")
	data, err = os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current checkpoint: %v", err)
	}
	if !strings.Contains(string(data), "status=completed") {
		t.Error("expected status in current checkpoint")
	}
}
