package state

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSnapshotStoreSaveAndLoadLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewSnapshotStore(root, "run-1")
	if err := store.Init("/tmp/plan.json", "checksum-1"); err != nil {
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
		PlanFile:     "/tmp/plan.json",
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

	loaded, path, err := LoadLatestSnapshot(root, "/tmp/plan.json")
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
