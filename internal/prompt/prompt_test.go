package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
)

func TestRenderExecutorSystem(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	t.Run("with project context", func(t *testing.T) {
		t.Parallel()
		got, err := engine.Render("executor.system", ExecutorSystemData{ProjectContext: "Go 1.26 project"})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !strings.Contains(got, "Go 1.26 project") {
			t.Errorf("expected project context in output, got:\n%s", got)
		}
		if !strings.Contains(got, "autonomous executor agent") {
			t.Errorf("expected role description in output, got:\n%s", got)
		}
	})

	t.Run("without project context", func(t *testing.T) {
		t.Parallel()
		got, err := engine.Render("executor.system", ExecutorSystemData{})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if strings.Contains(got, "Project Context") {
			t.Errorf("expected no project context header, got:\n%s", got)
		}
		if !strings.Contains(got, "autonomous executor agent") {
			t.Errorf("expected role description in output, got:\n%s", got)
		}
	})
}

func TestRenderExecutorTask(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	t.Run("basic task", func(t *testing.T) {
		t.Parallel()
		got, err := engine.Render("executor.task", ExecutorTaskData{
			TaskTitle:       "Add logging",
			TaskID:          "TASK-001",
			TaskIndex:       0,
			PlanFile:        "plan.json",
			PlanName:        "Main Plan",
			PlanProgress:    "1/3",
			Workdir:         "/tmp/work",
			TaskDescription: "Add structured logging",
			TaskAcceptance:  "- Tests pass",
		})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		for _, want := range []string{"Add logging", "TASK-001", "plan.json", "Main Plan", "1/3", "/tmp/work", "Add structured logging", "Tests pass"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %q in output, got:\n%s", want, got)
			}
		}
	})

	t.Run("retry", func(t *testing.T) {
		t.Parallel()
		got, err := engine.Render("executor.task", ExecutorTaskData{
			IsRetry:      true,
			RetryAttempt: 1,
			PreviousFeedback: []domain.TaskFeedback{{
				Attempt: 1,
				Phase:   "review",
				Verdict: "fail",
				Reason:  "tests failed",
				Hints:   []string{"fix the broken tests"},
			}},
			TaskTitle: "Fix bug",
			TaskID:    "TASK-002",
			TaskIndex: 1,
			PlanFile:  "plan.json",
			Workdir:   "/tmp/work",
		})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !strings.Contains(got, "RETRY") {
			t.Errorf("expected RETRY marker in output, got:\n%s", got)
		}
		if !strings.Contains(got, "attempt 2") {
			t.Errorf("expected attempt 2 in output, got:\n%s", got)
		}
		if !strings.Contains(got, "tests failed") {
			t.Errorf("expected feedback in output, got:\n%s", got)
		}
		if !strings.Contains(got, "fix the broken tests") {
			t.Errorf("expected hint in output, got:\n%s", got)
		}
	})
}

func TestRenderReviewerSystem(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := engine.Render("reviewer.system", ReviewerSystemData{ProjectContext: "test project"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "automated review gate") {
		t.Errorf("expected role description, got:\n%s", got)
	}
	if !strings.Contains(got, "test project") {
		t.Errorf("expected project context, got:\n%s", got)
	}
}

func TestRenderReviewerTask(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := engine.Render("reviewer.task", ReviewerTaskData{
		TaskTitle:      "Add logging",
		TaskID:         "TASK-001",
		PlanFile:       "plan.json",
		Workdir:        "/tmp/work",
		ExecutorOutput: "all tests passed",
		GitDiff:        "+added line",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"Add logging", "TASK-001", "all tests passed", "+added line"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestRenderPlannerSystem(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := engine.Render("planner.system", PlannerSystemData{ProjectContext: "Go project"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "planning agent") {
		t.Errorf("expected planning agent role, got:\n%s", got)
	}
	if !strings.Contains(got, "TASK-001") {
		t.Errorf("expected JSON schema example, got:\n%s", got)
	}
}

func TestRenderPlannerTask(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := engine.Render("planner.task", PlannerTaskData{Objective: "build a REST API"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "build a REST API") {
		t.Errorf("expected objective in output, got:\n%s", got)
	}
}

func TestRenderAdapterPlan(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	t.Run("with context", func(t *testing.T) {
		t.Parallel()
		got, err := engine.Render("adapter.plan", AdapterPlanData{
			Objective:        "build API",
			WorkspaceContext: "Go project with Cobra",
		})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if !strings.Contains(got, "Go project with Cobra") {
			t.Errorf("expected workspace context, got:\n%s", got)
		}
		if !strings.Contains(got, "build API") {
			t.Errorf("expected objective, got:\n%s", got)
		}
	})

	t.Run("without context", func(t *testing.T) {
		t.Parallel()
		got, err := engine.Render("adapter.plan", AdapterPlanData{Objective: "build API"})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if strings.Contains(got, "Project context") {
			t.Errorf("expected no context header, got:\n%s", got)
		}
	})
}

func TestRenderAdapterPlanClaude(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := engine.Render("adapter.plan.claude", AdapterPlanData{
		WorkspaceContext: "Go project",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "planning agent") {
		t.Errorf("expected planning agent role, got:\n%s", got)
	}
}

func TestOverlayOverridesDefault(t *testing.T) {
	t.Parallel()
	overlayDir := t.TempDir()
	custom := "CUSTOM EXECUTOR SYSTEM PROMPT for {{ .ProjectContext }}"
	if err := os.WriteFile(filepath.Join(overlayDir, "executor.system.tmpl"), []byte(custom), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	engine, err := NewEngine(overlayDir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	got, err := engine.Render("executor.system", ExecutorSystemData{ProjectContext: "my-project"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "CUSTOM EXECUTOR SYSTEM PROMPT for my-project") {
		t.Errorf("expected overridden template output, got:\n%s", got)
	}
	if strings.Contains(got, "autonomous executor agent") {
		t.Errorf("expected default content to be replaced, got:\n%s", got)
	}
}

func TestFallbackWhenOverlayDirMissing(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("/nonexistent/dir/that/does/not/exist")
	if err != nil {
		t.Fatalf("NewEngine should not error for missing overlay dir: %v", err)
	}
	got, err := engine.Render("executor.system", ExecutorSystemData{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "autonomous executor agent") {
		t.Errorf("expected default template, got:\n%s", got)
	}
}

func TestMissingKeyError(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	// Pass a map missing required keys to trigger missingkey=error.
	_, err = engine.Render("executor.system", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestRenderUnknownTemplate(t *testing.T) {
	t.Parallel()
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	_, err = engine.Render("nonexistent.template", nil)
	if err == nil {
		t.Fatal("expected error for unknown template, got nil")
	}
}

func TestOverlayPartialOverride(t *testing.T) {
	t.Parallel()
	overlayDir := t.TempDir()
	custom := "CUSTOM REVIEWER for {{ .ProjectContext }}"
	if err := os.WriteFile(filepath.Join(overlayDir, "reviewer.system.tmpl"), []byte(custom), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	engine, err := NewEngine(overlayDir)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Overridden template.
	got, err := engine.Render("reviewer.system", ReviewerSystemData{ProjectContext: "test"})
	if err != nil {
		t.Fatalf("Render reviewer.system: %v", err)
	}
	if !strings.Contains(got, "CUSTOM REVIEWER for test") {
		t.Errorf("expected custom reviewer, got:\n%s", got)
	}

	// Non-overridden template should still use default.
	got, err = engine.Render("executor.system", ExecutorSystemData{})
	if err != nil {
		t.Fatalf("Render executor.system: %v", err)
	}
	if !strings.Contains(got, "autonomous executor agent") {
		t.Errorf("expected default executor template, got:\n%s", got)
	}
}
