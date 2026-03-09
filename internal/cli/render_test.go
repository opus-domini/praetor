package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRendererConcurrentWrites(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	r := NewRenderer(&buf, true)

	var wg sync.WaitGroup
	const goroutines = 20
	const iterations = 50

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				switch j % 10 {
				case 0:
					r.Info("info from goroutine")
				case 1:
					r.Warn("warn from goroutine")
				case 2:
					r.Error("error from goroutine")
				case 3:
					r.Success("success from goroutine")
				case 4:
					r.Task("1/3", "TASK-001", "title")
				case 5:
					r.Phase("executor", "claude", "running")
				case 6:
					r.CheckItem("done", "TASK-001", "title")
				case 7:
					r.Blank()
				case 8:
					r.Dim("dim message")
				case 9:
					r.Hint("hint message")
				}
			}
		}(i)
	}
	wg.Wait()

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output from concurrent writes")
	}
	// Each line should be complete (no interleaving mid-line).
	// Since color is off (noColor=true), lines should contain readable text.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < goroutines {
		t.Fatalf("expected at least %d lines, got %d", goroutines, len(lines))
	}
}

func TestRendererConcurrentSummary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	r := NewRenderer(&buf, true)

	var wg sync.WaitGroup
	const goroutines = 10
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			r.Summary(3, 1, 5, 0.05, 30*time.Second)
		}()
	}
	wg.Wait()

	output := buf.String()
	count := strings.Count(output, "Run summary")
	if count != goroutines {
		t.Fatalf("expected %d summary lines, got %d", goroutines, count)
	}
}

func TestRendererNoColor(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	r := NewRenderer(&buf, true) // noColor = true

	r.Info("hello")
	r.Warn("careful")
	r.Error("oops")

	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Fatal("expected no ANSI escape codes when noColor=true")
	}
	if !strings.Contains(output, "[info]") {
		t.Fatal("expected [info] tag in output")
	}
	if !strings.Contains(output, "[warn]") {
		t.Fatal("expected [warn] tag in output")
	}
	if !strings.Contains(output, "[err]") {
		t.Fatal("expected [err] tag in output")
	}
}
