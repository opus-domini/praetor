package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/domain"
)

func TestResolvePlanCreateAgentSelectionUsesConfigDefaults(t *testing.T) {
	t.Parallel()

	got, err := resolvePlanCreateAgentSelection(config.Config{
		Planner:  "codex",
		Executor: "gemini",
		Reviewer: "none",
	}, "", "", "")
	if err != nil {
		t.Fatalf("resolvePlanCreateAgentSelection returned error: %v", err)
	}
	if got.Planner != domain.AgentCodex {
		t.Fatalf("planner = %q, want %q", got.Planner, domain.AgentCodex)
	}
	if got.Executor != domain.AgentGemini {
		t.Fatalf("executor = %q, want %q", got.Executor, domain.AgentGemini)
	}
	if got.Reviewer != domain.AgentNone {
		t.Fatalf("reviewer = %q, want %q", got.Reviewer, domain.AgentNone)
	}
}

func TestResolvePlanCreateAgentSelectionRejectsInvalidReviewer(t *testing.T) {
	t.Parallel()

	_, err := resolvePlanCreateAgentSelection(config.Config{}, "", "", "not-real")
	if err == nil {
		t.Fatal("expected invalid reviewer error")
	}
}

func TestPlanCreateWizardModelCompletesFlow(t *testing.T) {
	t.Parallel()

	model := newPlanCreateWizardModel(planCreateAgentSelection{
		Planner:  domain.AgentClaude,
		Executor: domain.AgentCodex,
		Reviewer: domain.AgentClaude,
	}, testPlanCreateAgentAvailability(
		healthyPlanCreateProbe(domain.AgentClaude),
		healthyPlanCreateProbe(domain.AgentCodex),
	))

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(planCreateWizardModel)
	if model.step != planCreateWizardStepExecutor {
		t.Fatalf("step after planner select = %v, want executor", model.step)
	}
	if model.result.Agents.Planner != domain.AgentClaude {
		t.Fatalf("planner = %q, want %q", model.result.Agents.Planner, domain.AgentClaude)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(planCreateWizardModel)
	if model.step != planCreateWizardStepReviewer {
		t.Fatalf("step after executor select = %v, want reviewer", model.step)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(planCreateWizardModel)
	if model.step != planCreateWizardStepBrief {
		t.Fatalf("step after reviewer select = %v, want brief", model.step)
	}

	model.brief.SetValue("Ship a guided plan creation flow.")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(planCreateWizardModel)
	if !model.done {
		t.Fatal("expected wizard to finish after ctrl+s")
	}
	if model.result.Brief != "Ship a guided plan creation flow." {
		t.Fatalf("brief = %q", model.result.Brief)
	}
}

func TestBuildPlanCreateAgentItemsFiltersHealthyProviders(t *testing.T) {
	t.Parallel()

	items, note := buildPlanCreateAgentItems(domain.ValidExecutors, false, testPlanCreateAgentAvailability(
		healthyPlanCreateProbe(domain.AgentClaude),
		healthyPlanCreateProbe(domain.AgentGemini),
		failedPlanCreateProbe(domain.AgentCodex, "codex not found in PATH"),
	))
	if !strings.Contains(note, "Showing only providers detected") {
		t.Fatalf("unexpected note %q", note)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	first := items[0].(planCreateAgentItem)
	second := items[1].(planCreateAgentItem)
	if first.value != domain.AgentClaude || second.value != domain.AgentGemini {
		t.Fatalf("filtered agents = [%q, %q]", first.value, second.value)
	}
}

func TestBuildPlanCreateAgentItemsFallsBackToAllWhenNothingHealthy(t *testing.T) {
	t.Parallel()

	items, note := buildPlanCreateAgentItems(domain.ValidReviewers, true, testPlanCreateAgentAvailability(
		failedPlanCreateProbe(domain.AgentClaude, "claude not found in PATH"),
		failedPlanCreateProbe(domain.AgentCodex, "codex not found in PATH"),
	))
	if !strings.Contains(note, "No healthy providers") {
		t.Fatalf("unexpected note %q", note)
	}
	if len(items) == 0 {
		t.Fatal("expected fallback items")
	}
	foundUnavailable := false
	foundNone := false
	for _, item := range items {
		candidate := item.(planCreateAgentItem)
		if candidate.value == domain.AgentNone {
			foundNone = true
		}
		if candidate.value == domain.AgentClaude && strings.Contains(candidate.description, "unavailable") {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatal("expected unavailable provider description in fallback mode")
	}
	if !foundNone {
		t.Fatal("expected no reviewer option in fallback mode")
	}
}

func TestPlanCreateNoAgentDryRunAppliesSelectedAgents(t *testing.T) {
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitCmd(t, repo, "init")
	t.Chdir(repo)

	root := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{
		"plan", "create", "Implement auth",
		"--no-agent",
		"--planner", "codex",
		"--executor", "gemini",
		"--reviewer", "none",
		"--dry-run",
		"--no-color",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute create command: %v (stderr=%s)", err, stderr.String())
	}

	out := stdout.String()
	start := strings.Index(out, "{")
	if start < 0 {
		t.Fatalf("expected JSON plan in output, got: %s", out)
	}

	var plan domain.Plan
	if err := json.Unmarshal([]byte(out[start:]), &plan); err != nil {
		t.Fatalf("decode plan preview: %v", err)
	}
	if plan.Settings.Agents.Planner.Agent != domain.AgentCodex {
		t.Fatalf("planner = %q, want %q", plan.Settings.Agents.Planner.Agent, domain.AgentCodex)
	}
	if plan.Settings.Agents.Executor.Agent != domain.AgentGemini {
		t.Fatalf("executor = %q, want %q", plan.Settings.Agents.Executor.Agent, domain.AgentGemini)
	}
	if plan.Settings.Agents.Reviewer.Agent != domain.AgentNone {
		t.Fatalf("reviewer = %q, want %q", plan.Settings.Agents.Reviewer.Agent, domain.AgentNone)
	}
}

func testPlanCreateAgentAvailability(results ...agent.ProbeResult) planCreateAgentAvailability {
	availability := planCreateAgentAvailability{results: make(map[domain.Agent]agent.ProbeResult, len(results))}
	for _, result := range results {
		availability.results[domain.NormalizeAgent(domain.Agent(result.ID))] = result
	}
	return availability
}

func healthyPlanCreateProbe(id domain.Agent) agent.ProbeResult {
	return agent.ProbeResult{
		ID:        agent.ID(id),
		Status:    agent.StatusPass,
		Transport: agent.TransportCLI,
		Version:   "1.2.3",
		Path:      "/tmp/" + string(id),
	}
}

func failedPlanCreateProbe(id domain.Agent, detail string) agent.ProbeResult {
	return agent.ProbeResult{
		ID:        agent.ID(id),
		Status:    agent.StatusFail,
		Transport: agent.TransportCLI,
		Detail:    detail,
	}
}
