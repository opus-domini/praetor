package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildPlannerRecoveryObjectiveIncludesStrictRulesAndObjective(t *testing.T) {
	t.Parallel()

	objective := "Criar um plano para refatorar o módulo X"
	previous := "Pronto. Arquivos criados com sucesso."

	recovery := buildPlannerRecoveryObjective(objective, previous)

	mustContain := []string{
		"STRICT RULES:",
		"Return ONE JSON object only.",
		"Do not execute actions.",
		"Original objective:",
		objective,
		"Previous invalid response:",
		previous,
	}
	for _, fragment := range mustContain {
		if !strings.Contains(recovery, fragment) {
			t.Fatalf("recovery prompt missing %q:\n%s", fragment, recovery)
		}
	}
}

func TestPlannerOutputPreviewUsesFirstLineAndTruncates(t *testing.T) {
	t.Parallel()

	longLine := strings.Repeat("a", 220)
	raw := longLine + "\nsecond line"
	preview := plannerOutputPreview(raw)

	if strings.Contains(preview, "\n") {
		t.Fatalf("preview should be single-line: %q", preview)
	}
	if len(preview) > 190 {
		t.Fatalf("preview should be truncated, got len=%d", len(preview))
	}
	if !strings.HasSuffix(preview, "...") {
		t.Fatalf("preview should end with ellipsis when truncated: %q", preview)
	}
}

func TestPlannerTimeoutError(t *testing.T) {
	t.Parallel()

	if err := plannerTimeoutError(context.DeadlineExceeded, 3*time.Minute); err == nil {
		t.Fatal("expected timeout error")
	}
	if err := plannerTimeoutError(errors.New("boom"), 3*time.Minute); err != nil {
		t.Fatalf("expected nil for non-timeout error, got: %v", err)
	}
	if err := plannerTimeoutError(context.DeadlineExceeded, 0); err != nil {
		t.Fatalf("expected nil when timeout is disabled, got: %v", err)
	}
}

func TestPlannerStartAndProgressMessagesIncludeAttempts(t *testing.T) {
	t.Parallel()

	start := plannerStartMessage(1, 2)
	if want := "Starting planner attempt 1/2..."; start != want {
		t.Fatalf("unexpected start message:\nwant: %q\ngot:  %q", want, start)
	}

	progress := plannerProgressMessage(2, 2, 81*time.Second, true)
	mustContain := []string{
		"Planner attempt 2/2 still running...",
		"elapsed 1m21s",
		"(Ctrl+C to cancel)",
	}
	for _, fragment := range mustContain {
		if !strings.Contains(progress, fragment) {
			t.Fatalf("progress message missing %q: %q", fragment, progress)
		}
	}
}

func TestPlannerRetryWarningMessageIncludesLogPathAndDuration(t *testing.T) {
	t.Parallel()

	message := plannerRetryWarningMessage(1, 2, 18*time.Minute, "/tmp/first-invalid.log")
	mustContain := []string{
		"Planner attempt 1/2 returned output without a valid JSON object after 18m0s.",
		"Raw output logged at /tmp/first-invalid.log.",
		"Retrying once with strict JSON instructions...",
	}
	for _, fragment := range mustContain {
		if !strings.Contains(message, fragment) {
			t.Fatalf("retry warning missing %q: %q", fragment, message)
		}
	}
}

func TestPlannerTotalDurationMessagePluralizesAttempts(t *testing.T) {
	t.Parallel()

	if got := plannerTotalDurationMessage(34*time.Second, 1); got != "Planner total duration: 34s across 1 attempt." {
		t.Fatalf("unexpected single-attempt message: %q", got)
	}
	if got := plannerTotalDurationMessage(2*time.Minute, 2); got != "Planner total duration: 2m0s across 2 attempts." {
		t.Fatalf("unexpected multi-attempt message: %q", got)
	}
}

func TestWritePlannerFailureLogUsesPlaceholderForEmptyOutput(t *testing.T) {
	t.Parallel()

	path, err := writePlannerFailureLog(t.TempDir(), "")
	if err != nil {
		t.Fatalf("writePlannerFailureLog returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read planner failure log: %v", err)
	}
	if string(data) != "<empty output>\n" {
		t.Fatalf("unexpected failure log contents: %q", string(data))
	}
}
