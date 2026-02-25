package claude

import (
	"bufio"
	"strings"
	"testing"
)

func TestBuildCLIArgsRejectsProtocolOverrideInExtraFlagArgs(t *testing.T) {
	t.Parallel()

	value := "text"
	_, err := buildCLIArgs(Options{
		ExtraFlagArgs: map[string]*string{
			"output-format": &value,
		},
	})
	if err == nil {
		t.Fatal("expected protocol override error")
	}
}

func TestBuildCLIArgsRejectsProtocolOverrideInExtraArgs(t *testing.T) {
	t.Parallel()

	_, err := buildCLIArgs(Options{
		ExtraArgs: []string{"--input-format=text"},
	})
	if err == nil {
		t.Fatal("expected protocol override error")
	}
}

func TestReadLineLimitedReturnsErrorWhenLineTooLarge(t *testing.T) {
	t.Parallel()

	reader := bufio.NewReader(strings.NewReader("0123456789\n"))
	_, err := readLineLimited(reader, 4)
	if err == nil {
		t.Fatal("expected line size error")
	}
}
