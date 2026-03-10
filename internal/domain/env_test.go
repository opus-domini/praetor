package domain

import (
	"os"
	"strings"
	"testing"
)

func TestCleanAgentEnvStripsNestingVars(t *testing.T) {
	// Set nesting vars for the test (can't use t.Parallel with t.Setenv).
	for _, name := range AgentNestingEnvVars {
		t.Setenv(name, "1")
	}

	cleaned := CleanAgentEnv(nil)
	for _, entry := range cleaned {
		for _, name := range AgentNestingEnvVars {
			if strings.HasPrefix(entry, name+"=") {
				t.Errorf("CleanAgentEnv should strip %s, but found %q", name, entry)
			}
		}
	}
}

func TestCleanAgentEnvPreservesOtherVars(t *testing.T) {
	t.Setenv("PRAETOR_TEST_MARKER", "keep-me")

	cleaned := CleanAgentEnv(nil)
	found := false
	for _, entry := range cleaned {
		if entry == "PRAETOR_TEST_MARKER=keep-me" {
			found = true
			break
		}
	}
	if !found {
		t.Error("CleanAgentEnv should preserve non-nesting variables")
	}
}

func TestCleanAgentEnvAppendsExtra(t *testing.T) {
	t.Parallel()

	extra := []string{"FOO=bar", "BAZ=qux"}
	cleaned := CleanAgentEnv(extra)

	// Last two entries should be our extras.
	if len(cleaned) < 2 {
		t.Fatal("expected at least 2 entries")
	}
	tail := cleaned[len(cleaned)-2:]
	if tail[0] != "FOO=bar" || tail[1] != "BAZ=qux" {
		t.Errorf("extra env not appended correctly: got %v", tail)
	}
}

func TestCleanAgentEnvReturnsFullEnvWhenNoNestingVars(t *testing.T) {
	// Ensure none of the nesting vars are set (can't use t.Parallel with env mutation).
	for _, name := range AgentNestingEnvVars {
		t.Setenv(name, "")
		os.Unsetenv(name) //nolint:errcheck // best-effort for test
	}

	base := os.Environ()
	cleaned := CleanAgentEnv(nil)

	if len(cleaned) != len(base) {
		t.Errorf("expected %d entries, got %d", len(base), len(cleaned))
	}
}

func TestAgentNestingEnvVarsIsNotEmpty(t *testing.T) {
	t.Parallel()
	if len(AgentNestingEnvVars) == 0 {
		t.Error("AgentNestingEnvVars should not be empty")
	}
}
