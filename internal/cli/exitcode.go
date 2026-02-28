package cli

import (
	"fmt"

	"github.com/opus-domini/praetor/internal/domain"
)

// ExitCoder exposes a process exit code to callers.
type ExitCoder interface {
	ExitCode() int
}

// ExitError carries a specific process exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("process exited with code %d", e.Code)
}

func (e ExitError) Unwrap() error {
	return e.Err
}

func (e ExitError) ExitCode() int {
	if e.Code <= 0 {
		return 1
	}
	return e.Code
}

func newExitError(code int, err error) error {
	if code <= 0 {
		return err
	}
	return ExitError{Code: code, Err: err}
}

func exitCodeForOutcome(outcome domain.RunOutcome) int {
	switch outcome {
	case domain.RunSuccess:
		return 0
	case domain.RunPartial:
		return 3
	case domain.RunCanceled:
		return 2
	case domain.RunFailed:
		return 1
	default:
		return 1
	}
}
