package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotStoreSaveAndLoadLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewSnapshotStore(root, "run-1")
	if err := store.Init("test-plan", "checksum-1"); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := store.WriteLock("token-1", 1234); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	statePayload, err := json.Marshal(map[string]any{"tasks": []string{"TASK-1"}})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := store.Save(Snapshot{
		RunID:        "run-1",
		PlanSlug:     "test-plan",
		PlanChecksum: "checksum-1",
		ProjectRoot:  root,
		Phase:        "execute",
		Message:      "ok",
		Iteration:    2,
		State:        statePayload,
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := store.AppendEvent(SnapshotEvent{Status: "execute", Message: "ok"}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	loaded, path, err := LoadLatestSnapshot(root, "test-plan")
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if path == "" {
		t.Fatal("expected snapshot path")
	}
	if filepath.Base(path) != "snapshot.json" {
		t.Fatalf("unexpected snapshot file: %s", path)
	}
	if loaded.RunID != "run-1" {
		t.Fatalf("unexpected run id: %s", loaded.RunID)
	}
	if loaded.Phase != "execute" {
		t.Fatalf("unexpected phase: %s", loaded.Phase)
	}
	if string(loaded.State) == "" {
		t.Fatal("expected state payload")
	}
}

func TestLoadLatestSnapshotSkipsChecksumMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewSnapshotStore(root, "run-1")
	if err := store.Init("test-plan", "checksum-1"); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := store.Save(Snapshot{
		RunID:        "run-1",
		PlanSlug:     "test-plan",
		PlanChecksum: "checksum-1",
		ProjectRoot:  root,
		Phase:        "execute",
		Message:      "ok",
		Iteration:    1,
		State:        json.RawMessage(`{"tasks":["TASK-1"]}`),
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	// Corrupt snapshot content while keeping metadata unchanged.
	snapshotPath := filepath.Join(store.RootDir(), "snapshot.json")
	if err := os.WriteFile(snapshotPath, []byte(`{"version":1,"run_id":"run-1","plan_slug":"test-plan","timestamp":"2026-02-26T00:00:00Z","state":{"corrupted":true}}`), 0o644); err != nil {
		t.Fatalf("corrupt snapshot: %v", err)
	}

	loaded, path, err := LoadLatestSnapshot(root, "test-plan")
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if path != "" {
		t.Fatalf("expected mismatched snapshot to be skipped, got %s", path)
	}
	if loaded.RunID != "" {
		t.Fatalf("expected empty snapshot, got run id %q", loaded.RunID)
	}
}

func TestPruneLocalSnapshotsKeepsLatestRuns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for i := range 3 {
		runID := "run-" + string(rune('a'+i))
		store := NewSnapshotStore(root, runID)
		if err := store.Init("test-plan", "checksum-1"); err != nil {
			t.Fatalf("init snapshot store %s: %v", runID, err)
		}
		ts := time.Now().UTC().Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		if err := store.Save(Snapshot{
			RunID:        runID,
			PlanSlug:     "test-plan",
			PlanChecksum: "checksum-1",
			ProjectRoot:  root,
			Phase:        "execute",
			Message:      "ok",
			Iteration:    i,
			Timestamp:    ts,
			State:        json.RawMessage(`{"tasks":[]}`),
		}); err != nil {
			t.Fatalf("save snapshot %s: %v", runID, err)
		}
	}

	if err := PruneLocalSnapshots(root, 2); err != nil {
		t.Fatalf("prune snapshots: %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read runtime root: %v", err)
	}
	// Filter to directories only (ignore any non-dir files).
	dirs := 0
	for _, e := range entries {
		if e.IsDir() {
			dirs++
		}
	}
	if dirs != 2 {
		t.Fatalf("expected 2 runtime dirs after prune, got %d", dirs)
	}
}
