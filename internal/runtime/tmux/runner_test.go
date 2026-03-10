package tmux

import (
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
)

func TestBuildWrapperScriptUnsetsAllNestingVars(t *testing.T) {
	t.Parallel()

	script := BuildWrapperScript(
		domain.CommandSpec{Args: []string{"echo", "test"}, Dir: "/tmp"},
		"", "stdout.log", "stderr.log", "exit.log", "chan-1", "",
	)

	// Verify each nesting var appears in the unset line.
	for _, name := range domain.AgentNestingEnvVars {
		if !strings.Contains(script, name) {
			t.Errorf("BuildWrapperScript missing unset for %s", name)
		}
	}

	// Verify there is a single unset line containing all vars.
	expected := "unset " + strings.Join(domain.AgentNestingEnvVars, " ")
	if !strings.Contains(script, expected) {
		t.Errorf("expected unset line %q in script, got:\n%s", expected, script)
	}
}

func TestBuildWrapperScriptContainsCommand(t *testing.T) {
	t.Parallel()

	script := BuildWrapperScript(
		domain.CommandSpec{Args: []string{"claude", "--print", "--json"}, Dir: "/tmp"},
		"", "stdout.log", "stderr.log", "exit.log", "chan-1", "",
	)

	if !strings.Contains(script, "'claude'") {
		t.Error("script should contain the command binary")
	}
	if !strings.Contains(script, "'--print'") {
		t.Error("script should contain command flags")
	}
}
