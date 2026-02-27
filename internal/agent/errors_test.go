package agent

import (
	"fmt"
	"testing"
)

func TestClassifyErrorNil(t *testing.T) {
	t.Parallel()
	if got := ClassifyError(nil); got != ErrorUnknown {
		t.Fatalf("expected unknown for nil error, got %q", got)
	}
}

func TestClassifyErrorTransient(t *testing.T) {
	t.Parallel()
	cases := []string{
		"connection refused",
		"dial tcp: connection refused",
		"request timeout",
		"HTTP 502 Bad Gateway",
		"status 503 service unavailable",
		"HTTP 504 gateway timeout",
		"temporary failure in name resolution",
		"network unreachable",
		"no such host",
		"connection reset by peer",
		"broken pipe",
		"unexpected eof",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(fmt.Errorf("%s", msg)); got != ErrorTransient {
				t.Fatalf("expected transient for %q, got %q", msg, got)
			}
		})
	}
}

func TestClassifyErrorAuth(t *testing.T) {
	t.Parallel()
	cases := []string{
		"HTTP 401 Unauthorized",
		"status 403 forbidden",
		"invalid api key",
		"unauthorized access",
		"authentication failed",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(fmt.Errorf("%s", msg)); got != ErrorAuth {
				t.Fatalf("expected auth for %q, got %q", msg, got)
			}
		})
	}
}

func TestClassifyErrorRateLimit(t *testing.T) {
	t.Parallel()
	cases := []string{
		"HTTP 429 Too Many Requests",
		"rate limit exceeded",
		"rate_limit_error",
		"too many requests",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(fmt.Errorf("%s", msg)); got != ErrorRateLimit {
				t.Fatalf("expected rate_limit for %q, got %q", msg, got)
			}
		})
	}
}

func TestClassifyErrorUnsupported(t *testing.T) {
	t.Parallel()
	cases := []string{
		"unsupported agent",
		"feature not implemented",
		"operation not supported",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(fmt.Errorf("%s", msg)); got != ErrorUnsupported {
				t.Fatalf("expected unsupported for %q, got %q", msg, got)
			}
		})
	}
}

func TestClassifyErrorUnknown(t *testing.T) {
	t.Parallel()
	cases := []string{
		"some random error",
		"task failed: exit code 1",
		"unexpected output format",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyError(fmt.Errorf("%s", msg)); got != ErrorUnknown {
				t.Fatalf("expected unknown for %q, got %q", msg, got)
			}
		})
	}
}

func TestClassifyErrorRateLimitTakesPrecedenceOverAuth(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("HTTP 429 Too Many Requests for unauthorized endpoint")
	if got := ClassifyError(err); got != ErrorRateLimit {
		t.Fatalf("expected rate_limit (higher priority), got %q", got)
	}
}
