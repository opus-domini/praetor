package pty

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestScriptSessionInteractiveWrite(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script command not available")
	}

	session := NewScriptSession()
	err := session.Start(context.Background(), CommandSpec{
		Args: []string{"/bin/sh", "-lc", "read line; echo OUT:$line; echo ERR:$line 1>&2"},
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	if err := session.Write("hello\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := session.CloseInput(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	var combined strings.Builder
	for ev := range session.Events() {
		combined.WriteString(ev.Data)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("wait session: %v", err)
	}
	if session.ExitCode() != 0 {
		t.Fatalf("unexpected exit code: %d", session.ExitCode())
	}

	output := combined.String()
	if !strings.Contains(output, "OUT:hello") {
		t.Fatalf("expected OUT:hello in output, got: %q", output)
	}
	if !strings.Contains(output, "ERR:hello") {
		t.Fatalf("expected ERR:hello in output, got: %q", output)
	}
}
