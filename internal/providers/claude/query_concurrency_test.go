package claude

import (
	"context"
	"errors"
	"testing"
)

func TestQueryCloseDoesNotPanicWithConcurrentErrorPush(t *testing.T) {
	t.Parallel()

	q := &Query{
		transport:      &processTransport{},
		msgCh:          make(chan SDKMessage, 1),
		errCh:          make(chan error, 1),
		done:           make(chan struct{}),
		pending:        make(map[string]chan controlResponse),
		incomingCancel: make(map[string]context.CancelFunc),
		hookCallbacks:  make(map[string]HookCallback),
		initDone:       make(chan struct{}),
	}

	q.workers.Add(1)
	go func() {
		defer q.workers.Done()
		q.pushErr(errors.New("concurrent error"))
	}()

	if err := q.Close(); err != nil {
		t.Fatalf("close returned error: %v", err)
	}
}
