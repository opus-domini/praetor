package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

func TestBuildPlanSelectionItemsIncludesMetadataAndStatus(t *testing.T) {
	t.Parallel()

	store := localstate.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	authPlan := domain.Plan{
		Name:    "Auth hardening",
		Summary: "Implement JWT auth, tests, and docs for the API.",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "Auth endpoints", Acceptance: []string{"login works"}},
			{ID: "TASK-002", Title: "Tests", Acceptance: []string{"coverage updated"}},
		},
	}
	writePlanJSON(t, store.PlanFile("auth"), authPlan)

	state := domain.State{
		PlanSlug:  "auth",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks:     domain.StateTasksFromPlan(authPlan),
	}
	state.Tasks[0].Status = domain.TaskDone
	if err := store.WriteState("auth", state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	billingPlan := domain.Plan{
		Name:    "Billing cleanup",
		Summary: "Refactor billing handlers.",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{ID: "TASK-001", Title: "Refactor", Acceptance: []string{"handlers simplified"}},
		},
	}
	writePlanJSON(t, store.PlanFile("billing"), billingPlan)

	items, err := buildPlanSelectionItems(store)
	if err != nil {
		t.Fatalf("build plan selection items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	first := items[0].(planSelectionItem)
	second := items[1].(planSelectionItem)

	if first.slug != "auth" {
		t.Fatalf("first slug = %q, want auth", first.slug)
	}
	if !strings.Contains(first.description, "Auth hardening") {
		t.Fatalf("first description missing plan name: %q", first.description)
	}
	if !strings.Contains(first.description, "in progress") {
		t.Fatalf("first description missing status: %q", first.description)
	}
	if !strings.Contains(first.description, "1/2 done") {
		t.Fatalf("first description missing progress: %q", first.description)
	}

	if second.slug != "billing" {
		t.Fatalf("second slug = %q, want billing", second.slug)
	}
	if !strings.Contains(second.description, "not started") {
		t.Fatalf("second description missing not started label: %q", second.description)
	}
}

func TestPlanSelectionModelSelectsCurrentItem(t *testing.T) {
	t.Parallel()

	model := newPlanSelectionModel("run", []list.Item{
		planSelectionItem{slug: "alpha", description: "Alpha plan"},
		planSelectionItem{slug: "beta", description: "Beta plan"},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(planSelectionModel)

	if !model.done {
		t.Fatal("expected selector to finish on enter")
	}
	if model.selectedSlug != "alpha" {
		t.Fatalf("selected slug = %q, want alpha", model.selectedSlug)
	}
}

func TestPlanSelectionModelCancelsOnEscape(t *testing.T) {
	t.Parallel()

	model := newPlanSelectionModel("run", []list.Item{
		planSelectionItem{slug: "alpha", description: "Alpha plan"},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(planSelectionModel)

	if !model.canceled {
		t.Fatal("expected selector to cancel on escape")
	}
}
