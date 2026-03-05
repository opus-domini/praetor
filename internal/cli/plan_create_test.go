package cli

import (
	"context"
	"errors"
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
