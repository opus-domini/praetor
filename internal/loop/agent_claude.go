package loop

import (
	claudeprovider "github.com/opus-domini/praetor/internal/providers/claude"
)

// claudeAgent delegates to providers/claude.AgentSpec.
type claudeAgent struct {
	inner claudeprovider.AgentSpec
}

func (a *claudeAgent) BuildCommand(req AgentRequest) (CommandSpec, error) {
	return a.inner.BuildCommand(req)
}

func (a *claudeAgent) ParseOutput(stdout string) (string, float64, error) {
	return a.inner.ParseOutput(stdout)
}

func (a *claudeAgent) String() string { return a.inner.String() }
