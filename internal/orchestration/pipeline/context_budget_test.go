package pipeline

import "testing"

func TestContextBudgetDefaults(t *testing.T) {
	t.Parallel()
	m := NewContextBudgetManager(0, 0)
	if m.Budget(promptPhaseExecute) != 120000 {
		t.Fatalf("unexpected execute default budget: %d", m.Budget(promptPhaseExecute))
	}
	if m.Budget(promptPhaseReview) != 80000 {
		t.Fatalf("unexpected review default budget: %d", m.Budget(promptPhaseReview))
	}
}

func TestContextBudgetTruncateReviewSections(t *testing.T) {
	t.Parallel()
	m := NewContextBudgetManager(1000, 100)
	overhead := "header"
	executorOutput := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	gitDiff := "diffdiffdiffdiffdiffdiffdiffdiffdiffdiffdiffdiffdiffdiffdiffdiff"

	execOut, diff, truncated := m.TruncateReviewSections(overhead, executorOutput, gitDiff)
	if len(execOut)+len(diff)+len(overhead) > 100 {
		t.Fatal("expected review prompt sections to fit budget")
	}
	if len(truncated) == 0 {
		t.Fatal("expected at least one truncated section")
	}
}

func TestContextBudgetTruncateExecuteFeedback(t *testing.T) {
	t.Parallel()
	m := NewContextBudgetManager(50, 50)
	feedback, truncated := m.TruncateExecuteFeedback("fixed prompt overhead with content", "very long feedback that should be truncated")
	if len(feedback)+len("fixed prompt overhead with content") > 50 {
		t.Fatal("expected feedback truncation to fit execute budget")
	}
	if len(truncated) == 0 {
		t.Fatal("expected feedback section to be marked truncated")
	}
}
