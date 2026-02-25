package cli

import "testing"

func TestRootCommandHasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()

	expected := map[string]bool{"run": false, "plan": false, "exec": false}
	for _, cmd := range root.Commands() {
		if _, ok := expected[cmd.Name()]; ok {
			expected[cmd.Name()] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestRootCommandHidesCompletion(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "completion" {
			t.Error("completion command should be disabled")
		}
	}
}
