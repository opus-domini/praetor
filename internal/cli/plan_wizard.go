package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/domain"
)

var errPlanCreateWizardCanceled = errors.New("plan creation canceled")

const (
	planCreateWizardProbeTimeout      = 1500 * time.Millisecond
	planCreateWizardProbeTotalTimeout = 4 * time.Second
)

type planCreateAgentSelection struct {
	Planner  domain.Agent
	Executor domain.Agent
	Reviewer domain.Agent
}

type planCreateWizardResult struct {
	Agents planCreateAgentSelection
	Brief  string
}

type planCreateWizardStep int

const (
	planCreateWizardStepPlanner planCreateWizardStep = iota
	planCreateWizardStepExecutor
	planCreateWizardStepReviewer
	planCreateWizardStepBrief
)

type planCreateAgentItem struct {
	value       domain.Agent
	title       string
	description string
}

func (i planCreateAgentItem) Title() string       { return i.title }
func (i planCreateAgentItem) Description() string { return i.description }
func (i planCreateAgentItem) FilterValue() string {
	return strings.ToLower(strings.TrimSpace(i.title + " " + i.description + " " + string(i.value)))
}

type planCreateWizardModel struct {
	step            planCreateWizardStep
	list            list.Model
	brief           textarea.Model
	width           int
	height          int
	validationError string
	done            bool
	canceled        bool
	result          planCreateWizardResult
	plannerItems    []list.Item
	executorItems   []list.Item
	reviewerItems   []list.Item
	plannerIndex    int
	executorIndex   int
	reviewerIndex   int
	plannerNote     string
	executorNote    string
	reviewerNote    string
}

type planCreateAgentAvailability struct {
	results map[domain.Agent]agent.ProbeResult
}

func resolvePlanCreateAgentSelection(cfg config.Config, plannerRaw, executorRaw, reviewerRaw string) (planCreateAgentSelection, error) {
	planner := strings.TrimSpace(plannerRaw)
	if planner == "" {
		planner = strings.TrimSpace(cfg.Planner)
	}
	if planner == "" {
		planner = string(domain.AgentClaude)
	}
	normalizedPlanner := domain.NormalizeAgent(domain.Agent(planner))
	if _, ok := domain.ValidExecutors[normalizedPlanner]; !ok {
		return planCreateAgentSelection{}, fmt.Errorf("invalid planner agent %q", planner)
	}

	executor := strings.TrimSpace(executorRaw)
	if executor == "" {
		executor = strings.TrimSpace(cfg.Executor)
	}
	if executor == "" {
		executor = string(domain.AgentCodex)
	}
	normalizedExecutor := domain.NormalizeAgent(domain.Agent(executor))
	if _, ok := domain.ValidExecutors[normalizedExecutor]; !ok {
		return planCreateAgentSelection{}, fmt.Errorf("invalid executor agent %q", executor)
	}

	reviewer := strings.TrimSpace(reviewerRaw)
	if reviewer == "" {
		reviewer = strings.TrimSpace(cfg.Reviewer)
	}
	if reviewer == "" {
		reviewer = string(domain.AgentClaude)
	}
	normalizedReviewer := domain.NormalizeAgent(domain.Agent(reviewer))
	if _, ok := domain.ValidReviewers[normalizedReviewer]; !ok {
		return planCreateAgentSelection{}, fmt.Errorf("invalid reviewer agent %q", reviewer)
	}

	return planCreateAgentSelection{
		Planner:  normalizedPlanner,
		Executor: normalizedExecutor,
		Reviewer: normalizedReviewer,
	}, nil
}

func shouldUsePlanCreateWizard(args []string, fromFile string, fromStdin bool, fromTemplate string, noAgent bool, stdin io.Reader, out io.Writer) bool {
	if noAgent || strings.TrimSpace(fromTemplate) != "" {
		return false
	}
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return false
	}
	if strings.TrimSpace(fromFile) != "" || fromStdin {
		return false
	}
	return isInteractiveInput(stdin) && isInteractiveOutput(out)
}

func runPlanCreateWizard(ctx context.Context, stdin io.Reader, out io.Writer, defaults planCreateAgentSelection, cfg config.Config) (planCreateWizardResult, error) {
	input, ok := stdin.(*os.File)
	if !ok {
		return planCreateWizardResult{}, errors.New("interactive wizard requires terminal stdin")
	}
	output, ok := out.(*os.File)
	if !ok {
		return planCreateWizardResult{}, errors.New("interactive wizard requires terminal stdout")
	}

	model := newPlanCreateWizardModel(defaults, detectPlanCreateAgentAvailability(ctx, cfg))
	finalModel, err := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return planCreateWizardResult{}, err
	}

	wizard, ok := finalModel.(planCreateWizardModel)
	if !ok {
		return planCreateWizardResult{}, errors.New("interactive wizard returned an unexpected model")
	}
	if wizard.canceled {
		return planCreateWizardResult{}, errPlanCreateWizardCanceled
	}
	if !wizard.done {
		return planCreateWizardResult{}, errors.New("interactive wizard exited without a result")
	}
	return wizard.result, nil
}

func newPlanCreateWizardModel(defaults planCreateAgentSelection, availability planCreateAgentAvailability) planCreateWizardModel {
	delegate := list.NewDefaultDelegate()
	selector := list.New(nil, delegate, 84, 12)
	selector.DisableQuitKeybindings()
	selector.SetShowHelp(false)
	selector.SetShowFilter(false)
	selector.SetShowPagination(false)
	selector.SetShowStatusBar(false)
	selector.SetFilteringEnabled(false)

	brief := textarea.New()
	brief.Placeholder = "Describe the desired plan, goals, constraints, and expected outcomes."
	brief.Prompt = "│ "
	brief.ShowLineNumbers = false
	brief.CharLimit = 0
	brief.SetWidth(84)
	brief.SetHeight(12)

	plannerItems, plannerNote := buildPlanCreateAgentItems(domain.ValidExecutors, false, availability)
	executorItems, executorNote := buildPlanCreateAgentItems(domain.ValidExecutors, false, availability)
	reviewerItems, reviewerNote := buildPlanCreateAgentItems(domain.ValidReviewers, true, availability)

	m := planCreateWizardModel{
		step:  planCreateWizardStepPlanner,
		list:  list.New(nil, delegate, 84, 12),
		brief: brief,
		result: planCreateWizardResult{
			Agents: defaults,
		},
		plannerItems:  plannerItems,
		executorItems: executorItems,
		reviewerItems: reviewerItems,
		plannerNote:   plannerNote,
		executorNote:  executorNote,
		reviewerNote:  reviewerNote,
		width:         92,
		height:        24,
	}
	m.list = selector
	m.plannerIndex = planCreateAgentIndex(defaults.Planner, m.plannerItems)
	m.executorIndex = planCreateAgentIndex(defaults.Executor, m.executorItems)
	m.reviewerIndex = planCreateAgentIndex(defaults.Reviewer, m.reviewerItems)
	m.configureCurrentStep()
	return m
}

func (m planCreateWizardModel) Init() tea.Cmd {
	return nil
}

func (m planCreateWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.canceled = true
			return m, tea.Quit
		}
		switch m.step {
		case planCreateWizardStepBrief:
			return m.updateBrief(msg)
		default:
			return m.updateList(msg)
		}
	}

	switch m.step {
	case planCreateWizardStepBrief:
		var cmd tea.Cmd
		m.brief, cmd = m.brief.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
}

func (m planCreateWizardModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Praetor Plan Wizard\n\n")
	fmt.Fprintf(&b, "Planner:  %s\n", planCreateWizardSelectionLabel(m.result.Agents.Planner))
	fmt.Fprintf(&b, "Executor: %s\n", planCreateWizardSelectionLabel(m.result.Agents.Executor))
	fmt.Fprintf(&b, "Reviewer: %s\n\n", planCreateWizardSelectionLabel(m.result.Agents.Reviewer))

	switch m.step {
	case planCreateWizardStepPlanner, planCreateWizardStepExecutor, planCreateWizardStepReviewer:
		if note := strings.TrimSpace(m.currentStepNote()); note != "" {
			b.WriteString(note)
			b.WriteString("\n\n")
		}
		b.WriteString(m.list.View())
		b.WriteString("\n")
		if m.step == planCreateWizardStepPlanner {
			b.WriteString("enter select  •  esc cancel  •  ctrl+c cancel")
		} else {
			b.WriteString("enter select  •  esc back  •  ctrl+c cancel")
		}
	case planCreateWizardStepBrief:
		b.WriteString("Write the plan brief.\n")
		b.WriteString(m.brief.View())
		b.WriteString("\n")
		if strings.TrimSpace(m.validationError) != "" {
			b.WriteString(m.validationError)
			b.WriteString("\n")
		}
		b.WriteString("ctrl+s submit  •  esc back  •  ctrl+c cancel")
	}
	return b.String()
}

func (m planCreateWizardModel) currentStepNote() string {
	switch m.step {
	case planCreateWizardStepPlanner:
		return m.plannerNote
	case planCreateWizardStepExecutor:
		return m.executorNote
	case planCreateWizardStepReviewer:
		return m.reviewerNote
	default:
		return ""
	}
}

func (m planCreateWizardModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.advanceFromListSelection()
	case tea.KeyEsc:
		if m.step == planCreateWizardStepPlanner {
			m.canceled = true
			return m, tea.Quit
		}
		m.moveToPreviousListStep()
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m planCreateWizardModel) updateBrief(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.validationError = ""
		m.step = planCreateWizardStepReviewer
		m.brief.Blur()
		m.configureCurrentStep()
		return m, nil
	case tea.KeyCtrlS:
		brief := strings.TrimSpace(m.brief.Value())
		if brief == "" {
			m.validationError = "Plan brief cannot be empty."
			return m, nil
		}
		m.result.Brief = brief
		m.done = true
		return m, tea.Quit
	}

	if msg.Type == tea.KeyRunes || msg.Type == tea.KeyEnter || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
		m.validationError = ""
	}
	var cmd tea.Cmd
	m.brief, cmd = m.brief.Update(msg)
	return m, cmd
}

func (m *planCreateWizardModel) advanceFromListSelection() (tea.Model, tea.Cmd) {
	selected, ok := m.list.SelectedItem().(planCreateAgentItem)
	if !ok {
		return *m, nil
	}
	switch m.step {
	case planCreateWizardStepPlanner:
		m.plannerIndex = m.list.GlobalIndex()
		m.result.Agents.Planner = selected.value
		m.step = planCreateWizardStepExecutor
		m.configureCurrentStep()
		return *m, nil
	case planCreateWizardStepExecutor:
		m.executorIndex = m.list.GlobalIndex()
		m.result.Agents.Executor = selected.value
		m.step = planCreateWizardStepReviewer
		m.configureCurrentStep()
		return *m, nil
	case planCreateWizardStepReviewer:
		m.reviewerIndex = m.list.GlobalIndex()
		m.result.Agents.Reviewer = selected.value
		m.step = planCreateWizardStepBrief
		m.configureCurrentStep()
		return *m, m.brief.Focus()
	default:
		return *m, nil
	}
}

func (m *planCreateWizardModel) moveToPreviousListStep() {
	switch m.step {
	case planCreateWizardStepExecutor:
		m.step = planCreateWizardStepPlanner
	case planCreateWizardStepReviewer:
		m.step = planCreateWizardStepExecutor
	}
	m.configureCurrentStep()
}

func (m *planCreateWizardModel) configureCurrentStep() {
	switch m.step {
	case planCreateWizardStepPlanner:
		m.list.Title = "1/4 Choose the planner provider"
		m.list.SetItems(m.plannerItems)
		m.list.Select(m.plannerIndex)
	case planCreateWizardStepExecutor:
		m.list.Title = "2/4 Choose the executor provider"
		m.list.SetItems(m.executorItems)
		m.list.Select(m.executorIndex)
	case planCreateWizardStepReviewer:
		m.list.Title = "3/4 Choose the reviewer provider"
		m.list.SetItems(m.reviewerItems)
		m.list.Select(m.reviewerIndex)
	case planCreateWizardStepBrief:
		m.brief.Focus()
	}
	m.resize()
}

func (m *planCreateWizardModel) resize() {
	width := m.width
	if width <= 0 {
		width = 92
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	listWidth := maxInt(width-4, 60)
	listHeight := maxInt(height-10, 8)
	m.list.SetSize(listWidth, listHeight)
	m.brief.SetWidth(maxInt(width-4, 60))
	m.brief.SetHeight(maxInt(height-12, 8))
}

func planCreateWizardSelectionLabel(a domain.Agent) string {
	if a == domain.AgentNone {
		return "No Reviewer"
	}
	display := strings.TrimSpace(domain.AgentDisplayName(a))
	if display == "" {
		return string(a)
	}
	return fmt.Sprintf("%s (%s)", display, a)
}

func buildPlanCreateAgentItems(allowed map[domain.Agent]struct{}, includeNone bool, availability planCreateAgentAvailability) ([]list.Item, string) {
	order := []domain.Agent{
		domain.AgentClaude,
		domain.AgentCodex,
		domain.AgentCopilot,
		domain.AgentGemini,
		domain.AgentKimi,
		domain.AgentOpenCode,
		domain.AgentOpenRouter,
		domain.AgentOllama,
		domain.AgentLMStudio,
	}
	healthyCount := 0
	for _, id := range order {
		if _, ok := allowed[id]; !ok {
			continue
		}
		if availability.Healthy(id) {
			healthyCount++
		}
	}

	items := make([]list.Item, 0, len(order)+1)
	includeAll := healthyCount == 0
	for _, id := range order {
		if _, ok := allowed[id]; !ok {
			continue
		}
		if !includeAll && !availability.Healthy(id) {
			continue
		}
		items = append(items, planCreateAgentItem{
			value:       id,
			title:       planCreateAgentTitle(id),
			description: availability.Description(id, includeAll),
		})
	}
	if includeNone {
		items = append(items, planCreateAgentItem{
			value:       domain.AgentNone,
			title:       "No Reviewer",
			description: "none • skip the review gate for this plan",
		})
	}
	if includeAll {
		return items, "No healthy providers were detected for this role. Showing all known providers."
	}
	return items, "Showing only providers detected as available on this machine."
}

func planCreateAgentIndex(target domain.Agent, items []list.Item) int {
	target = domain.NormalizeAgent(target)
	for i, item := range items {
		candidate, ok := item.(planCreateAgentItem)
		if !ok {
			continue
		}
		if candidate.value == target {
			return i
		}
	}
	return 0
}

func isInteractiveOutput(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func composePlanCreateProjectContext(projectContext string, agents planCreateAgentSelection) string {
	parts := make([]string, 0, 2)
	projectContext = strings.TrimSpace(projectContext)
	if projectContext != "" {
		parts = append(parts, projectContext)
	}

	lines := []string{
		"## Execution Defaults",
		fmt.Sprintf("- planner: %s", strings.TrimSpace(planCreateWizardSelectionLabel(agents.Planner))),
		fmt.Sprintf("- executor: %s", strings.TrimSpace(planCreateWizardSelectionLabel(agents.Executor))),
		fmt.Sprintf("- reviewer: %s", strings.TrimSpace(planCreateWizardSelectionLabel(agents.Reviewer))),
		"Use these defaults in the generated plan settings unless the objective explicitly requires a per-task override.",
	}
	parts = append(parts, strings.Join(lines, "\n"))
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func detectPlanCreateAgentAvailability(ctx context.Context, cfg config.Config) planCreateAgentAvailability {
	probeCtx, cancel := context.WithTimeout(ctx, planCreateWizardProbeTotalTimeout)
	defer cancel()

	binaryOverrides := buildBinaryOverrides(cfg)
	restEndpoints := buildRESTEndpoints(cfg)
	prober := agent.NewProber(agent.WithTimeout(planCreateWizardProbeTimeout))

	entries := agent.AllCatalogEntries()
	results := make(map[domain.Agent]agent.ProbeResult, len(entries))

	type probeItem struct {
		id     domain.Agent
		result agent.ProbeResult
	}

	out := make(chan probeItem, len(entries))
	var wg sync.WaitGroup
	for _, entry := range entries {
		wg.Add(1)
		go func(entry agent.CatalogEntry) {
			defer wg.Done()
			result, _ := prober.ProbeOne(probeCtx, entry.ID, binaryOverrides[entry.ID], restEndpoints[entry.ID])
			out <- probeItem{
				id:     domain.NormalizeAgent(domain.Agent(entry.ID)),
				result: result,
			}
		}(entry)
	}

	wg.Wait()
	close(out)

	for item := range out {
		results[item.id] = item.result
	}
	return planCreateAgentAvailability{results: results}
}

func (a planCreateAgentAvailability) Healthy(id domain.Agent) bool {
	result, ok := a.Result(id)
	return ok && result.Healthy()
}

func (a planCreateAgentAvailability) Result(id domain.Agent) (agent.ProbeResult, bool) {
	if a.results == nil {
		return agent.ProbeResult{}, false
	}
	result, ok := a.results[domain.NormalizeAgent(id)]
	return result, ok
}

func (a planCreateAgentAvailability) Description(id domain.Agent, includeUnavailable bool) string {
	if result, ok := a.Result(id); ok {
		if result.Healthy() {
			return planCreateHealthyDescription(id, result)
		}
		if includeUnavailable {
			return fmt.Sprintf("unavailable • %s", shortenPlanCreateDescription(strings.TrimSpace(result.Detail), 64, "not detected on this machine"))
		}
	}

	parts := []string{string(id)}
	if caps, ok := domain.CapabilitiesForAgent(id); ok {
		parts = append(parts, string(caps.Transport))
		if caps.RequiresTTY {
			parts = append(parts, "tty")
		}
	}
	return strings.Join(parts, " • ")
}

func planCreateHealthyDescription(id domain.Agent, result agent.ProbeResult) string {
	parts := []string{"available"}
	if caps, ok := domain.CapabilitiesForAgent(id); ok {
		parts = append(parts, string(caps.Transport))
	}
	if version := strings.TrimSpace(result.Version); version != "" {
		parts = append(parts, "v"+version)
	}
	if result.Transport == agent.TransportREST {
		if path := strings.TrimSpace(result.Path); path != "" {
			parts = append(parts, shortenPlanCreateDescription(path, 36, path))
		}
	}
	return strings.Join(parts, " • ")
}

func planCreateAgentTitle(id domain.Agent) string {
	title := strings.TrimSpace(domain.AgentDisplayName(id))
	if title == "" {
		return string(id)
	}
	return title
}

func shortenPlanCreateDescription(value string, maxLen int, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	if maxLen <= 3 || len(value) <= maxLen {
		return value
	}
	return strings.TrimSpace(value[:maxLen-3]) + "..."
}
