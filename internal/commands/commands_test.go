package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCreatesCommandsAndSymlinks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := Sync(dir, []string{"claude", "cursor"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Check commands directory exists with files.
	commandsDir := filepath.Join(dir, ".agents", "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		t.Fatalf("read commands dir: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 commands, got %d", len(entries))
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "praetor-") {
			t.Fatalf("expected prefixed command file, got %q", name)
		}
	}

	// Check symlinks.
	for _, agent := range []string{"claude", "cursor"} {
		link := filepath.Join(dir, "."+agent, "commands")
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("readlink for %s: %v", agent, err)
		}
		if target != filepath.Join("..", ".agents", "commands") {
			t.Fatalf("expected relative symlink, got %q", target)
		}
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := Sync(dir, []string{"claude"}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := Sync(dir, []string{"claude"}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	names, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 5 {
		t.Fatalf("expected 5 commands, got %d", len(names))
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "praetor-") {
			t.Fatalf("expected prefixed command name, got %q", name)
		}
	}
}

func TestListReturnsEmptyForMissingDir(t *testing.T) {
	t.Parallel()
	names, err := List(t.TempDir())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty list, got %d", len(names))
	}
}
