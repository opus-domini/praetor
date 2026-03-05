package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommandHasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()

	expected := map[string]bool{"plan": false, "eval": false, "exec": false, "doctor": false, "config": false, "mcp": false, "init": false}
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

func TestRunIsSubcommandOfPlan(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()

	// "run" should NOT be a root-level command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "run" {
			t.Error("run should not be a root-level subcommand; it belongs under plan")
		}
	}

	// "run" should be a subcommand of "plan"
	var planCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "plan" {
			planCmd = cmd
			break
		}
	}
	if planCmd == nil {
		t.Fatal("plan command not found")
	}

	found := false
	for _, cmd := range planCmd.Commands() {
		if cmd.Name() == "run" {
			found = true
			break
		}
	}
	if !found {
		t.Error("run command not found under plan")
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

func TestMissingArgsShowsUsageAndError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantUsage string // expected in stdout (usage)
		wantError string // expected in stderr (error message)
	}{
		{"plan status", []string{"plan", "status"}, "status <slug>", "Error:"},
		{"plan reset", []string{"plan", "reset"}, "reset <slug>", "Error:"},
		{"plan resume", []string{"plan", "resume"}, "resume <slug>", "Error:"},
		{"plan run", []string{"plan", "run"}, "run <slug>", "Error:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tt.args)

			err := root.Execute()
			if err == nil {
				t.Fatal("expected error for missing args")
			}

			usageOutput := stdout.String()
			if tt.wantUsage != "" && !bytes.Contains([]byte(usageOutput), []byte(tt.wantUsage)) {
				t.Errorf("expected stdout to contain usage %q, got:\n%s", tt.wantUsage, usageOutput)
			}

			errOutput := stderr.String()
			if !bytes.Contains([]byte(errOutput), []byte(tt.wantError)) {
				t.Errorf("expected stderr to contain %q, got:\n%s", tt.wantError, errOutput)
			}
		})
	}
}
