package loop

import (
	codexprovider "github.com/opus-domini/praetor/internal/providers/codex"
)

// codexAgent delegates to providers/codex.AgentSpec.
type codexAgent struct {
	inner codexprovider.AgentSpec
}

func (a *codexAgent) BuildCommand(req AgentRequest) (CommandSpec, error) {
	return a.inner.BuildCommand(req)
}

func (a *codexAgent) ParseOutput(stdout string) (string, float64, error) {
	return a.inner.ParseOutput(stdout)
}

func (a *codexAgent) String() string { return a.inner.String() }
