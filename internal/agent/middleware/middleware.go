package middleware

import (
	"context"

	"github.com/opus-domini/praetor/internal/domain"
)

// Middleware wraps an AgentRuntime to add cross-cutting behavior.
// Same pattern as http.Handler middleware — each wraps the next.
type Middleware func(next domain.AgentRuntime) domain.AgentRuntime

// Chain applies middlewares in order, outermost first.
// Chain(base, A, B) produces A(B(base)).
func Chain(base domain.AgentRuntime, mws ...Middleware) domain.AgentRuntime {
	rt := base
	for i := len(mws) - 1; i >= 0; i-- {
		rt = mws[i](rt)
	}
	return rt
}

// runtimeFunc is a convenience adapter for simple middleware implementations.
type runtimeFunc func(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error)

func (f runtimeFunc) Run(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
	return f(ctx, req)
}
