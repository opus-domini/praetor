package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestProjectEvalRejectsInvalidWindow(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"eval", "--window", "not-a-duration"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected invalid duration error")
	}
	if !strings.Contains(err.Error(), "invalid --window duration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectEvalRejectsInvalidFormat(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"eval", "--format", "xml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected format error")
	}
	if !strings.Contains(err.Error(), "allowed: table, json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlanEvalRejectsInvalidFormat(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"plan", "eval", "demo", "--format", "yaml"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected format error")
	}
	if !strings.Contains(err.Error(), "allowed: table, json") {
		t.Fatalf("unexpected error: %v", err)
	}
}
