package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

func TestPlanCreateFromTemplateCreatesPlanFile(t *testing.T) {
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitCmd(t, repo, "init")
	t.Chdir(repo)

	root := NewRootCmd()
	root.SetArgs([]string{
		"plan", "create",
		"--from-template", "go-feature",
		"--var", "Name=Auth API",
		"--var", "Summary=Implement JWT authentication",
		"--var", "Description=Implement JWT authentication end-to-end",
		"--no-color",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute create command: %v", err)
	}

	projectHome, err := localstate.ResolveProjectHome("", repo)
	if err != nil {
		t.Fatalf("resolve project home: %v", err)
	}
	store := localstate.NewStore(projectHome)
	plan, err := domain.LoadPlan(store.PlanFile("auth-api"))
	if err != nil {
		t.Fatalf("load generated plan: %v", err)
	}
	if plan.Name != "Auth API" {
		t.Fatalf("plan name = %q, want %q", plan.Name, "Auth API")
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(plan.Tasks))
	}
	if plan.Meta.Source != "template:go-feature" {
		t.Fatalf("plan source = %q, want %q", plan.Meta.Source, "template:go-feature")
	}
}

func TestPlanCreateFromTemplateFailsOnMissingVariable(t *testing.T) {
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitCmd(t, repo, "init")
	t.Chdir(repo)

	root := NewRootCmd()
	root.SetArgs([]string{
		"plan", "create",
		"--from-template", "go-feature",
		"--var", "Name=Auth API",
		"--no-color",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected template render error")
	}
	if !strings.Contains(err.Error(), "Summary") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlanExportWritesBundle(t *testing.T) {
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitCmd(t, repo, "init")
	t.Chdir(repo)

	projectHome, err := localstate.ResolveProjectHome("", repo)
	if err != nil {
		t.Fatalf("resolve project home: %v", err)
	}
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	slug := "demo-plan"
	plan := domain.Plan{
		Name:    "Demo Plan",
		Summary: "Ship the demo plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "Implement Demo Plan", Description: "Ship the demo plan", Acceptance: []string{"done"}},
		},
	}
	writePlanJSON(t, store.PlanFile(slug), plan)

	checksum, err := domain.PlanChecksum(store.PlanFile(slug))
	if err != nil {
		t.Fatalf("plan checksum: %v", err)
	}
	state := domain.State{
		PlanSlug:        slug,
		PlanChecksum:    checksum,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		ExecutionPolicy: plan.Settings.ExecutionPolicy,
		TotalCostMicros: 2_500_000,
		Tasks:           domain.StateTasksFromPlan(plan),
	}
	state.Tasks[0].Status = domain.TaskDone
	if err := store.WriteState(slug, state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	snapshots := localstate.NewLocalSnapshotStore(store.RuntimeDir(), "run-1")
	if err := snapshots.Init(slug, checksum); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := snapshots.Save(localstate.LocalSnapshot{
		RunID:        "run-1",
		PlanSlug:     slug,
		PlanChecksum: checksum,
		ProjectRoot:  repo,
		Phase:        "finalize",
		Message:      "completed",
		Outcome:      domain.RunSuccess,
		Iteration:    1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Summary: domain.RunSummary{
			TotalCostUSD: 2.5,
			TasksDone:    1,
			ByActor: map[string]domain.ActorStats{
				"executor:codex": {CostUSD: 2.5, Calls: 1},
			},
		},
		State: state,
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	exportDir := filepath.Join(repo, "bundle")
	root := NewRootCmd()
	root.SetArgs([]string{"plan", "export", slug, "--output", exportDir})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute export command: %v", err)
	}

	for _, name := range []string{"plan.json", "state.json", "summary.json", "template.json"} {
		if _, err := os.Stat(filepath.Join(exportDir, name)); err != nil {
			t.Fatalf("expected exported file %s: %v", name, err)
		}
	}

	summaryBytes, err := os.ReadFile(filepath.Join(exportDir, "summary.json"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(summaryBytes, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if got := summary["total_cost_usd"]; got != float64(2.5) {
		t.Fatalf("total_cost_usd = %v, want 2.5", got)
	}

	templateBytes, err := os.ReadFile(filepath.Join(exportDir, "template.json"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	content := string(templateBytes)
	if !strings.Contains(content, "{{.Name}}") {
		t.Fatalf("template missing Name placeholder: %s", content)
	}
	if !strings.Contains(content, "{{.Summary}}") {
		t.Fatalf("template missing Summary placeholder: %s", content)
	}
}
