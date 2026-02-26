package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunEchoHello(t *testing.T) {
	t.Parallel()

	r := &Runner{}
	res, err := r.Run(context.Background(), CommandSpec{
		Args: []string{"echo", "hello"},
	}, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if res.Stdout != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", res.Stdout)
	}
}

func TestRunNonExistentCommand(t *testing.T) {
	t.Parallel()

	r := &Runner{}
	_, err := r.Run(context.Background(), CommandSpec{
		Args: []string{"this-command-does-not-exist-anywhere"},
	}, "", "")
	if err == nil {
		t.Fatal("expected error for non-existent command, got nil")
	}
}

func TestRunEmptyCommand(t *testing.T) {
	t.Parallel()

	r := &Runner{}
	_, err := r.Run(context.Background(), CommandSpec{}, "", "")
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

func TestRunNonZeroExit(t *testing.T) {
	t.Parallel()

	r := &Runner{}
	res, err := r.Run(context.Background(), CommandSpec{
		Args: []string{"sh", "-c", "exit 42"},
	}, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", res.ExitCode)
	}
}

func TestRunStdin(t *testing.T) {
	t.Parallel()

	r := &Runner{}
	res, err := r.Run(context.Background(), CommandSpec{
		Args:  []string{"cat"},
		Stdin: "from stdin",
	}, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "from stdin" {
		t.Fatalf("expected stdout %q, got %q", "from stdin", res.Stdout)
	}
}

func TestRunPersistsOutput(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	r := &Runner{}
	res, err := r.Run(context.Background(), CommandSpec{
		Args: []string{"sh", "-c", "echo out; echo err >&2"},
	}, runDir, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Stdout != "out" {
		t.Fatalf("expected stdout %q, got %q", "out", res.Stdout)
	}
	if res.Stderr != "err" {
		t.Fatalf("expected stderr %q, got %q", "err", res.Stderr)
	}

	// Verify files were written.
	for _, name := range []string{"test.stdout", "test.stderr"} {
		path := filepath.Join(runDir, name)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("failed to read %s: %v", name, readErr)
		}
		if len(data) == 0 {
			t.Fatalf("expected non-empty %s", name)
		}
	}
}
