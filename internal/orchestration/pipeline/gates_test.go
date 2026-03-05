package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHostGateRunnerRun(t *testing.T) {
	t.Parallel()

	runner := NewHostGateRunner(map[string]string{
		"tests": "go test ./...",
		"lint":  "golangci-lint run",
	})
	runner.execFn = func(_ context.Context, _ string, command string) (string, error) {
		switch command {
		case "go test ./...":
			return "ok", nil
		case "golangci-lint run":
			return "2 issues", errors.New("exit status 1")
		default:
			return "", errors.New("unexpected command")
		}
	}

	results := runner.Run(context.Background(), ".", []string{"tests", "lint", "standards"}, []string{"custom"}, 0)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].Status != gateStatusPass {
		t.Fatalf("tests status = %s, want PASS", results[0].Status)
	}
	if results[1].Status != gateStatusFail {
		t.Fatalf("lint status = %s, want FAIL", results[1].Status)
	}
	if results[2].Status != gateStatusMissing {
		t.Fatalf("standards status = %s, want MISSING", results[2].Status)
	}
	if results[3].Status != gateStatusMissing {
		t.Fatalf("custom status = %s, want MISSING", results[3].Status)
	}
	if results[3].Detail == "" {
		t.Fatal("expected custom gate missing detail")
	}
}

func TestHostGateRunnerTimeout(t *testing.T) {
	t.Parallel()

	runner := NewHostGateRunner(map[string]string{
		"tests": "go test ./...",
	})
	runner.execFn = func(ctx context.Context, _ string, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}

	results := runner.Run(context.Background(), ".", []string{"tests"}, nil, 5*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != gateStatusError {
		t.Fatalf("status = %s, want ERROR", results[0].Status)
	}
}
