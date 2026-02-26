package fsm

import "context"

// StateFn is a state function that receives context and mutable state,
// performs its work, and returns the next state function (or nil to stop).
// This adapts Rob Pike's lexer pattern for serializable orchestration.
type StateFn[S any] func(ctx context.Context, state *S) (StateFn[S], error)

// Run drives the state machine from an initial state function until
// it returns nil (completion) or an error.
func Run[S any](ctx context.Context, state *S, initial StateFn[S]) error {
	fn := initial
	for fn != nil {
		next, err := fn(ctx, state)
		if err != nil {
			return err
		}
		fn = next
	}
	return nil
}
