package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadManifestPrefersYAMLOverMarkdown(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "praetor.md"), []byte("md context"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "praetor.yaml"), []byte("scope: yaml"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if got := filepath.Base(manifest.Path); got != "praetor.yaml" {
		t.Fatalf("expected praetor.yaml, got %s", got)
	}
	if !strings.Contains(manifest.Context, "```yaml") {
		t.Fatalf("expected YAML fenced context, got %q", manifest.Context)
	}
}

func TestReadManifestTruncatesLargeContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	large := strings.Repeat("a", MaxManifestSize+256)
	if err := os.WriteFile(filepath.Join(root, "praetor.md"), []byte(large), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !manifest.Truncated {
		t.Fatal("expected truncated manifest")
	}
	if len(manifest.Context) != MaxManifestSize {
		t.Fatalf("expected truncated length %d, got %d", MaxManifestSize, len(manifest.Context))
	}
}
