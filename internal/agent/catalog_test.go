package agent

import (
	"testing"
)

func TestAllCatalogEntriesCoversAllKnownAgents(t *testing.T) {
	t.Parallel()

	expected := []ID{Claude, Codex, Copilot, Gemini, Kimi, Ollama, OpenCode, OpenRouter}
	entries := AllCatalogEntries()

	if len(entries) != len(expected) {
		t.Fatalf("expected %d catalog entries, got %d", len(expected), len(entries))
	}

	// AllCatalogEntries returns sorted by ID.
	for i, entry := range entries {
		if entry.ID != expected[i] {
			t.Errorf("entry[%d]: expected ID %q, got %q", i, expected[i], entry.ID)
		}
	}
}

func TestLookupCatalogFoundAndNotFound(t *testing.T) {
	t.Parallel()

	entry, ok := LookupCatalog(Claude)
	if !ok {
		t.Fatal("expected Claude to be in catalog")
	}
	if entry.DisplayName != "Claude Code" {
		t.Errorf("expected DisplayName 'Claude Code', got %q", entry.DisplayName)
	}
	if entry.Transport != TransportCLI {
		t.Errorf("expected transport CLI, got %q", entry.Transport)
	}

	_, ok = LookupCatalog("nonexistent")
	if ok {
		t.Fatal("expected nonexistent agent to not be in catalog")
	}
}

func TestLookupCatalogNormalizesID(t *testing.T) {
	t.Parallel()

	entry, ok := LookupCatalog("  Claude  ")
	if !ok {
		t.Fatal("expected normalized lookup to find Claude")
	}
	if entry.ID != Claude {
		t.Errorf("expected ID %q, got %q", Claude, entry.ID)
	}

	entry2, ok := LookupCatalog("OLLAMA")
	if !ok {
		t.Fatal("expected normalized lookup to find Ollama")
	}
	if entry2.ID != Ollama {
		t.Errorf("expected ID %q, got %q", Ollama, entry2.ID)
	}
}

func TestCLICatalogEntriesOnlyContainsCLI(t *testing.T) {
	t.Parallel()

	entries := CLICatalogEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one CLI entry")
	}
	for _, entry := range entries {
		if entry.Transport != TransportCLI {
			t.Errorf("CLICatalogEntries returned non-CLI entry: %s (transport=%s)", entry.ID, entry.Transport)
		}
	}

	expectedCLI := map[ID]bool{Claude: true, Codex: true, Copilot: true, Gemini: true, Kimi: true, OpenCode: true}
	for _, entry := range entries {
		if !expectedCLI[entry.ID] {
			t.Errorf("unexpected CLI entry: %s", entry.ID)
		}
	}
	if len(entries) != len(expectedCLI) {
		t.Errorf("expected %d CLI entries, got %d", len(expectedCLI), len(entries))
	}
}

func TestRESTCatalogEntriesOnlyContainsREST(t *testing.T) {
	t.Parallel()

	entries := RESTCatalogEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one REST entry")
	}
	for _, entry := range entries {
		if entry.Transport != TransportREST {
			t.Errorf("RESTCatalogEntries returned non-REST entry: %s (transport=%s)", entry.ID, entry.Transport)
		}
	}

	expectedREST := map[ID]bool{Ollama: true, OpenRouter: true}
	for _, entry := range entries {
		if !expectedREST[entry.ID] {
			t.Errorf("unexpected REST entry: %s", entry.ID)
		}
	}
	if len(entries) != len(expectedREST) {
		t.Errorf("expected %d REST entries, got %d", len(expectedREST), len(entries))
	}
}

func TestCatalogIDsSorted(t *testing.T) {
	t.Parallel()

	ids := CatalogIDs()
	if len(ids) != 8 {
		t.Fatalf("expected 8 catalog IDs, got %d", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("CatalogIDs not sorted: %q <= %q at position %d", ids[i], ids[i-1], i)
		}
	}
}

func TestCatalogEntryFieldsNonEmpty(t *testing.T) {
	t.Parallel()

	for _, entry := range AllCatalogEntries() {
		t.Run(string(entry.ID), func(t *testing.T) {
			if entry.DisplayName == "" {
				t.Error("DisplayName is empty")
			}
			if entry.Transport == "" {
				t.Error("Transport is empty")
			}
			if entry.InstallHint == "" {
				t.Error("InstallHint is empty")
			}

			if entry.Transport == TransportCLI {
				if entry.Binary == "" {
					t.Error("CLI agent must have a Binary")
				}
				if len(entry.VersionArgs) == 0 {
					t.Error("CLI agent should have VersionArgs for health checks")
				}
			}

			if entry.Transport == TransportREST {
				if entry.Binary != "" {
					t.Error("REST agent should not have a Binary")
				}
				if entry.HealthEndpoint == "" {
					t.Error("REST agent should have a HealthEndpoint")
				}
				if entry.DefaultBaseURL == "" {
					t.Error("REST agent should have a DefaultBaseURL")
				}
			}
		})
	}
}

func TestCatalogCapabilitiesMatchTransport(t *testing.T) {
	t.Parallel()

	for _, entry := range AllCatalogEntries() {
		t.Run(string(entry.ID), func(t *testing.T) {
			if entry.Capabilities.Transport != entry.Transport {
				t.Errorf("entry transport %q != capabilities transport %q", entry.Transport, entry.Capabilities.Transport)
			}
			// All agents must support at least Execute.
			if !entry.Capabilities.SupportsExecute {
				t.Error("every agent must support Execute")
			}
		})
	}
}

func TestIsSupportedUsesTheCatalog(t *testing.T) {
	t.Parallel()

	supported := []ID{Claude, Codex, Copilot, Gemini, Kimi, OpenCode, OpenRouter, Ollama}
	for _, id := range supported {
		if !IsSupported(id) {
			t.Errorf("expected %q to be supported", id)
		}
	}

	if IsSupported("nonexistent") {
		t.Error("expected nonexistent to not be supported")
	}
	if IsSupported(None) {
		t.Error("expected 'none' to not be supported")
	}
}
