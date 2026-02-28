package config

import (
	"strings"
	"testing"
)

func TestTemplateContainsAllKeys(t *testing.T) {
	t.Parallel()

	tmpl := Template()
	for _, meta := range Registry {
		if !strings.Contains(tmpl, meta.Key) {
			t.Errorf("template missing key %q", meta.Key)
		}
	}
}

func TestTemplateAllKeysCommentedOut(t *testing.T) {
	t.Parallel()

	tmpl := Template()
	for _, line := range strings.Split(tmpl, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			t.Errorf("expected all lines commented, found: %q", line)
		}
	}
}

func TestTemplateContainsCategoryHeaders(t *testing.T) {
	t.Parallel()

	tmpl := Template()
	for _, cat := range CategoryOrder {
		header := "# === " + string(cat) + " ==="
		if !strings.Contains(tmpl, header) {
			t.Errorf("template missing category header %q", header)
		}
	}
}

func TestTemplateContainsDescriptions(t *testing.T) {
	t.Parallel()

	tmpl := Template()
	for _, meta := range Registry {
		if !strings.Contains(tmpl, meta.Description) {
			t.Errorf("template missing description for %q: %q", meta.Key, meta.Description)
		}
	}
}
