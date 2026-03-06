package state

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireTaskLockPreventsConcurrentOwnership(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "home"))
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	lock, err := store.AcquireTaskLock("demo", "TASK-001", false)
	if err != nil {
		t.Fatalf("acquire first task lock: %v", err)
	}
	defer func() { _ = store.ReleaseTaskLock(lock) }()

	_, err = store.AcquireTaskLock("demo", "TASK-001", false)
	if err == nil {
		t.Fatal("expected second task lock acquisition to fail")
	}
	if !strings.Contains(err.Error(), "already held") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseTaskLockRejectsWrongToken(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "home"))
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	lock, err := store.AcquireTaskLock("demo", "TASK-001", false)
	if err != nil {
		t.Fatalf("acquire task lock: %v", err)
	}
	defer func() { _ = store.ReleaseTaskLock(lock) }()

	err = store.ReleaseTaskLock(TaskLock{Path: lock.Path, Token: "wrong-token"})
	if err == nil {
		t.Fatal("expected ownership mismatch error")
	}
	if !strings.Contains(err.Error(), "ownership mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReleaseTaskLockIgnoresMissingFile(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "home"))
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if err := store.ReleaseTaskLock(TaskLock{}); err != nil {
		t.Fatalf("release empty task lock: %v", err)
	}
	if err := store.ReleaseTaskLock(TaskLock{Path: filepath.Join(store.LocksDir(), "missing.lock"), Token: "x"}); err != nil {
		t.Fatalf("release missing task lock: %v", err)
	}
}
