package claude

import (
	"context"
	"fmt"
)

// Prompt is a one-shot helper that sends one user prompt and waits for result.
func Prompt(ctx context.Context, prompt string, opts Options) (SDKMessage, error) {
	q, err := NewQuery(ctx, opts)
	if err != nil {
		return SDKMessage{}, err
	}
	defer func() {
		_ = q.Close()
	}()

	if err := q.WaitInitialized(ctx); err != nil {
		return SDKMessage{}, fmt.Errorf("initialization failed: %w", err)
	}
	if err := q.SendUserText(prompt); err != nil {
		return SDKMessage{}, err
	}
	if err := q.EndInput(); err != nil {
		return SDKMessage{}, err
	}
	return q.AwaitResult(ctx)
}
