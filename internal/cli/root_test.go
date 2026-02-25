package cli

import "testing"

func TestKnownProvidersSorted(t *testing.T) {
	t.Parallel()

	ids := knownProviders()
	if len(ids) != 2 {
		t.Fatalf("unexpected provider count: %d", len(ids))
	}
	if ids[0] != "claude" || ids[1] != "codex" {
		t.Fatalf("unexpected providers order: %v", ids)
	}
}
