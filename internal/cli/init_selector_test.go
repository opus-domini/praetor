package cli

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInitSelectorModelDetectsExistingAgents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	model := newInitSelectorModel(dir)

	for _, item := range model.items {
		if item.name == "claude" {
			if !item.detected {
				t.Error("claude should be detected")
			}
			if !item.selected {
				t.Error("claude should be selected (detected)")
			}
		} else {
			if item.detected {
				t.Errorf("%s should not be detected", item.name)
			}
			if item.selected {
				t.Errorf("%s should not be selected (not detected)", item.name)
			}
		}
	}
}

func TestInitSelectorModelDefaultsAllWhenNoneDetected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	for _, item := range model.items {
		if !item.selected {
			t.Errorf("%s should be selected by default when none detected", item.name)
		}
	}
}

func TestInitSelectorToggle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	// All selected by default. Toggle first item off.
	initial := model.items[0].selected
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	m := updated.(initSelectorModel)
	if m.items[0].selected == initial {
		t.Error("space should toggle selection")
	}

	// Toggle it back.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(initSelectorModel)
	if m.items[0].selected != initial {
		t.Error("second space should toggle back")
	}
}

func TestInitSelectorToggleAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	// All selected. Press 'a' to deselect all.
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m := updated.(initSelectorModel)
	for _, item := range m.items {
		if item.selected {
			t.Errorf("%s should be deselected after toggle all", item.name)
		}
	}

	// Press 'a' again to select all.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(initSelectorModel)
	for _, item := range m.items {
		if !item.selected {
			t.Errorf("%s should be selected after toggle all", item.name)
		}
	}
}

func TestInitSelectorNavigation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	if model.cursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", model.cursor)
	}

	// Move down.
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	m := updated.(initSelectorModel)
	if m.cursor != 1 {
		t.Errorf("cursor should be 1 after down, got %d", m.cursor)
	}

	// Move up.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(initSelectorModel)
	if m.cursor != 0 {
		t.Errorf("cursor should be 0 after up, got %d", m.cursor)
	}

	// Can't go above 0.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(initSelectorModel)
	if m.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", m.cursor)
	}
}

func TestInitSelectorEnterSubmits(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m := updated.(initSelectorModel)
	if !m.done {
		t.Error("enter should set done=true")
	}
}

func TestInitSelectorEscCancels(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m := updated.(initSelectorModel)
	if !m.canceled {
		t.Error("esc should set canceled=true")
	}
}

func TestInitSelectorViewRenders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	model := newInitSelectorModel(dir)

	view := model.View()
	if view == "" {
		t.Fatal("view should not be empty")
	}

	// Check all agent names appear.
	for _, item := range model.items {
		if !containsString(view, item.displayName) {
			t.Errorf("view missing agent %q", item.displayName)
		}
	}

	// Check keybinding hints.
	if !containsString(view, "space toggle") {
		t.Error("view missing space hint")
	}
	if !containsString(view, "enter confirm") {
		t.Error("view missing enter hint")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
