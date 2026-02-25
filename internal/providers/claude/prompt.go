package claude

import "context"

// Prompt is a one-shot helper that sends one user prompt and waits for result.
func Prompt(ctx context.Context, prompt string, opts Options) (SDKMessage, error) {
	q, err := NewQuery(ctx, opts)
	if err != nil {
		return SDKMessage{}, err
	}
	defer q.Close()

	if err := q.SendUserText(prompt); err != nil {
		return SDKMessage{}, err
	}
	if err := q.EndInput(); err != nil {
		return SDKMessage{}, err
	}
	return q.AwaitResult(ctx)
}
