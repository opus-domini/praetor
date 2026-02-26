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
	if strings.TrimSpace(manifest.RawContext) != "scope: yaml" {
		t.Fatalf("expected raw context to preserve yaml content, got %q", manifest.RawContext)
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
	if len(manifest.RawContext) != MaxManifestSize {
		t.Fatalf("expected truncated raw context length %d, got %d", MaxManifestSize, len(manifest.RawContext))
	}
}

func TestReadManifestNormalizesStructuredYAML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := strings.Join([]string{
		"version: \"1\"",
		"instructions:",
		"  - follow coding standards",
		"constraints:",
		"  - do not change public API",
		"test_commands:",
		"  - go test ./...",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "praetor.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(manifest.Context, "## Workspace Manifest") {
		t.Fatalf("expected normalized manifest context, got %q", manifest.Context)
	}
	if strings.TrimSpace(manifest.Hash) == "" {
		t.Fatal("expected non-empty manifest hash")
	}
	if !strings.Contains(manifest.RawContext, "instructions:") {
		t.Fatalf("expected raw yaml context, got %q", manifest.RawContext)
	}
}

func TestReadManifestFallsBackToMarkdown(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "praetor.md"), []byte("md context"), 0o644); err != nil {
		t.Fatalf("write md: %v", err)
	}

	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if got := filepath.Base(manifest.Path); got != "praetor.md" {
		t.Fatalf("expected praetor.md, got %s", got)
	}
	if strings.TrimSpace(manifest.Context) != "md context" {
		t.Fatalf("unexpected markdown context: %q", manifest.Context)
	}
	if strings.TrimSpace(manifest.RawContext) != "md context" {
		t.Fatalf("unexpected markdown raw context: %q", manifest.RawContext)
	}
}

func TestReadManifestReturnsEmptyWhenMissing(t *testing.T) {
	t.Parallel()

	manifest, err := ReadManifest(t.TempDir())
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.TrimSpace(manifest.Path) != "" {
		t.Fatalf("expected empty manifest path, got %q", manifest.Path)
	}
	if strings.TrimSpace(manifest.Context) != "" {
		t.Fatalf("expected empty manifest context, got %q", manifest.Context)
	}
}

func TestReadManifestInvalidYAMLFallsBackToFencedRaw(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := strings.Join([]string{
		"version: \"1\"",
		"unknown_key: true",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "praetor.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	manifest, err := ReadManifest(root)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(manifest.Context, "```yaml") {
		t.Fatalf("expected fenced yaml fallback, got %q", manifest.Context)
	}
	if strings.TrimSpace(manifest.RawContext) != strings.TrimSpace(content) {
		t.Fatalf("expected raw yaml preserved, got %q", manifest.RawContext)
	}
}
