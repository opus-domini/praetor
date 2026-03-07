package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/spf13/cobra"
)

var errPlanSelectionCanceled = errors.New("plan selection canceled")

type planSelectionMetadata struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type planSelectionItem struct {
	slug        string
	name        string
	summary     string
	description string
}

func (i planSelectionItem) Title() string {
	return i.slug
}

func (i planSelectionItem) Description() string {
	return i.description
}

func (i planSelectionItem) FilterValue() string {
	return strings.ToLower(strings.TrimSpace(strings.Join([]string{i.slug, i.name, i.summary, i.description}, " ")))
}

type planSelectionModel struct {
	action       string
	list         list.Model
	width        int
	height       int
	done         bool
	canceled     bool
	selectedSlug string
}

func planSlugArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && isInteractiveInput(cmd.InOrStdin()) && isInteractiveOutput(cmd.OutOrStdout()) {
		return nil
	}
	return cobra.ExactArgs(1)(cmd, args)
}

func resolvePlanSlug(cmd *cobra.Command, store *localstate.Store, args []string, action string) (string, error) {
	if len(args) > 0 {
		if slug := strings.TrimSpace(args[0]); slug != "" {
			return slug, nil
		}
	}

	if !isInteractiveInput(cmd.InOrStdin()) || !isInteractiveOutput(cmd.OutOrStdout()) {
		return "", errors.New("plan slug is required")
	}
	return runPlanSelector(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), store, action)
}

func runPlanSelector(ctx context.Context, stdin io.Reader, out io.Writer, store *localstate.Store, action string) (string, error) {
	input, ok := stdin.(*os.File)
	if !ok {
		return "", errors.New("interactive plan selector requires terminal stdin")
	}
	output, ok := out.(*os.File)
	if !ok {
		return "", errors.New("interactive plan selector requires terminal stdout")
	}

	items, err := buildPlanSelectionItems(store)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no plans found in %s", store.PlansDir())
	}

	model := newPlanSelectionModel(action, items)
	finalModel, err := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return "", err
	}

	selector, ok := finalModel.(planSelectionModel)
	if !ok {
		return "", errors.New("interactive plan selector returned an unexpected model")
	}
	if selector.canceled {
		return "", errPlanSelectionCanceled
	}
	if !selector.done || strings.TrimSpace(selector.selectedSlug) == "" {
		return "", errors.New("interactive plan selector exited without a selection")
	}
	return selector.selectedSlug, nil
}

func buildPlanSelectionItems(store *localstate.Store) ([]list.Item, error) {
	slugs, err := store.ListPlanSlugs()
	if err != nil {
		return nil, err
	}
	if len(slugs) == 0 {
		return nil, nil
	}

	statuses, err := store.ListPlanStatuses()
	if err != nil {
		return nil, err
	}
	statusBySlug := make(map[string]domain.PlanStatus, len(statuses))
	for _, status := range statuses {
		statusBySlug[status.PlanSlug] = status
	}

	items := make([]list.Item, 0, len(slugs))
	for _, slug := range slugs {
		meta := readPlanSelectionMetadata(store.PlanFile(slug))
		descriptionParts := make([]string, 0, 3)
		if name := strings.TrimSpace(meta.Name); name != "" {
			descriptionParts = append(descriptionParts, name)
		}
		if status, ok := statusBySlug[slug]; ok {
			descriptionParts = append(descriptionParts, planSelectionStatusSummary(status))
		}
		if summary := shortenPlanSelectionText(meta.Summary, 96); summary != "" {
			descriptionParts = append(descriptionParts, summary)
		}
		description := strings.Join(descriptionParts, " • ")
		if description == "" {
			description = "available"
		}
		items = append(items, planSelectionItem{
			slug:        slug,
			name:        strings.TrimSpace(meta.Name),
			summary:     strings.TrimSpace(meta.Summary),
			description: description,
		})
	}
	return items, nil
}

func readPlanSelectionMetadata(path string) planSelectionMetadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return planSelectionMetadata{}
	}
	var meta planSelectionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return planSelectionMetadata{}
	}
	return meta
}

func planSelectionStatusSummary(status domain.PlanStatus) string {
	label := planStatusLabel(status)
	if status.Total <= 0 {
		return label
	}
	return fmt.Sprintf("%s • %d/%d done", label, status.Done, status.Total)
}

func shortenPlanSelectionText(value string, maxLen int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" || maxLen <= 3 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen-3] + "..."
}

func newPlanSelectionModel(action string, items []list.Item) planSelectionModel {
	delegate := list.NewDefaultDelegate()
	selector := list.New(items, delegate, 84, 14)
	selector.DisableQuitKeybindings()
	selector.Title = fmt.Sprintf("Select a plan to %s", strings.TrimSpace(action))
	selector.SetShowHelp(false)
	selector.SetShowFilter(true)
	selector.SetShowStatusBar(true)
	selector.SetShowPagination(false)
	selector.SetFilteringEnabled(true)
	selector.SetStatusBarItemName("plan", "plans")

	model := planSelectionModel{
		action: action,
		list:   selector,
		width:  92,
		height: 24,
	}
	model.resize()
	return model
}

func (m planSelectionModel) Init() tea.Cmd {
	return nil
}

func (m planSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.canceled = true
			return m, tea.Quit
		case tea.KeyEnter:
			if !m.list.SettingFilter() {
				selected, ok := m.list.SelectedItem().(planSelectionItem)
				if ok {
					m.selectedSlug = selected.slug
					m.done = true
					return m, tea.Quit
				}
			}
		case tea.KeyEsc:
			if m.list.FilterState() == list.Unfiltered {
				m.canceled = true
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m planSelectionModel) View() string {
	var b strings.Builder
	b.WriteString("Praetor Plan Selector\n\n")
	b.WriteString(m.list.View())
	b.WriteString("\n")
	b.WriteString("enter select  •  / filter  •  esc cancel  •  ctrl+c cancel")
	return b.String()
}

func (m *planSelectionModel) resize() {
	width := m.width
	if width <= 0 {
		width = 92
	}
	height := m.height
	if height <= 0 {
		height = 24
	}
	m.list.SetSize(maxInt(width-4, 60), maxInt(height-8, 10))
}
