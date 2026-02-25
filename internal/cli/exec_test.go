package cli

import (
	"strings"
	"testing"
)

func TestReadPromptFromFlag(t *testing.T) {
	t.Parallel()

	prompt, err := readPrompt("  hello  ", strings.NewReader("ignored"), false)
	if err != nil {
		t.Fatalf("readPrompt returned error: %v", err)
	}
	if prompt != "hello" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestReadPromptFromStdin(t *testing.T) {
	t.Parallel()

	prompt, err := readPrompt("", strings.NewReader("  from stdin  "), false)
	if err != nil {
		t.Fatalf("readPrompt returned error: %v", err)
	}
	if prompt != "from stdin" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestReadPromptRequiresInput(t *testing.T) {
	t.Parallel()

	_, err := readPrompt("", strings.NewReader("\n\n"), false)
	if err == nil {
		t.Fatalf("expected input validation error")
	}
}

func TestReadPromptRejectsInteractiveStdinWithoutPrompt(t *testing.T) {
	t.Parallel()

	_, err := readPrompt("", strings.NewReader(""), true)
	if err == nil {
		t.Fatal("expected interactive stdin validation error")
	}
}
