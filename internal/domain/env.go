package domain

import (
	"os"
	"strings"
)

// AgentNestingEnvVars lists environment variables set by AI agent CLI tools
// to detect nested sessions. Praetor strips these from child processes so
// spawned agents (e.g. claude, codex) start normally when orchestrated.
var AgentNestingEnvVars = []string{
	"CLAUDECODE",
	"CLAUDE_CODE",
	"CODEX_SANDBOX",
}

// CleanAgentEnv returns os.Environ() with nesting-detection variables removed
// and any extra key=value pairs appended.
func CleanAgentEnv(extra []string) []string {
	base := os.Environ()
	cleaned := make([]string, 0, len(base)+len(extra))
	for _, entry := range base {
		skip := false
		for _, name := range AgentNestingEnvVars {
			if strings.HasPrefix(entry, name+"=") {
				skip = true
				break
			}
		}
		if !skip {
			cleaned = append(cleaned, entry)
		}
	}
	return append(cleaned, extra...)
}
