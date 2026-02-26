package cli

import (
	"strings"
	"testing"
)

func TestFormatExecMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		provider  string
		model     string
		durationS float64
		costUSD   float64
		want      string
	}{
		{
			name:      "full metadata",
			provider:  "claude",
			model:     "sonnet",
			durationS: 12.3,
			costUSD:   0.0042,
			want:      "provider=claude model=sonnet duration=12.3s cost=$0.0042",
		},
		{
			name:      "omit model when empty",
			provider:  "codex",
			model:     "",
			durationS: 5.0,
			costUSD:   0.01,
			want:      "provider=codex duration=5.0s cost=$0.0100",
		},
		{
			name:      "omit cost when zero",
			provider:  "ollama",
			model:     "llama3.1",
			durationS: 2.5,
			costUSD:   0,
			want:      "provider=ollama model=llama3.1 duration=2.5s",
		},
		{
			name:      "omit both model and cost",
			provider:  "codex",
			model:     "",
			durationS: 1.0,
			costUSD:   0,
			want:      "provider=codex duration=1.0s",
		},
		{
			name:      "include cost when positive",
			provider:  "claude",
			model:     "opus",
			durationS: 30.0,
			costUSD:   0.15,
			want:      "provider=claude model=opus duration=30.0s cost=$0.1500",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatExecMeta(tt.provider, tt.model, tt.durationS, tt.costUSD)
			if got != tt.want {
				t.Errorf("formatExecMeta() = %q, want %q", got, tt.want)
			}
		})
	}
}

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
