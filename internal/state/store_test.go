package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesBriefsDir(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "home"))
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	briefsDir := store.BriefsDir()
	info, err := os.Stat(briefsDir)
	if err != nil {
		t.Fatalf("stat briefs dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("briefs path is not a directory")
	}
}

func TestBriefsDirPath(t *testing.T) {
	t.Parallel()

	store := NewStore("/tmp/test-home")
	expected := filepath.Join("/tmp/test-home", "briefs")
	if got := store.BriefsDir(); got != expected {
		t.Fatalf("BriefsDir: expected %q, got %q", expected, got)
	}
}

func TestBriefFilePath(t *testing.T) {
	t.Parallel()

	store := NewStore("/tmp/test-home")
	expected := filepath.Join("/tmp/test-home", "briefs", "20260309-120000-abc.md")
	if got := store.BriefFile("20260309-120000-abc.md"); got != expected {
		t.Fatalf("BriefFile: expected %q, got %q", expected, got)
	}
}
