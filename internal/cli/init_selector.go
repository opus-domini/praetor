package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/opus-domini/praetor/internal/commands"
)

var errInitSelectorCanceled = errors.New("agent selection canceled")

type initAgentItem struct {
	name        string
	displayName string
	detected    bool
	selected    bool
}

type initSelectorModel struct {
	items    []initAgentItem
	cursor   int
	width    int
	height   int
	done     bool
	canceled bool
}

func shouldUseInitSelector(stdin io.Reader, out io.Writer) bool {
	return isInteractiveInput(stdin) && isInteractiveOutput(out)
}

func runInitSelector(stdin io.Reader, out io.Writer, projectRoot string) ([]string, error) {
	input, ok := stdin.(*os.File)
	if !ok {
		return nil, errors.New("interactive selector requires terminal stdin")
	}
	output, ok := out.(*os.File)
	if !ok {
		return nil, errors.New("interactive selector requires terminal stdout")
	}

	model := newInitSelectorModel(projectRoot)
	finalModel, err := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return nil, err
	}

	selector, ok := finalModel.(initSelectorModel)
	if !ok {
		return nil, errors.New("interactive selector returned an unexpected model")
	}
	if selector.canceled {
		return nil, errInitSelectorCanceled
	}
	if !selector.done {
		return nil, errors.New("interactive selector exited without a result")
	}

	var selected []string
	for _, item := range selector.items {
		if item.selected {
			selected = append(selected, item.name)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no agents selected")
	}
	return selected, nil
}

func newInitSelectorModel(projectRoot string) initSelectorModel {
	detected := detectAgents(projectRoot)
	detectedSet := make(map[string]bool, len(detected))
	for _, name := range detected {
		detectedSet[name] = true
	}

	items := make([]initAgentItem, 0, len(commands.SupportedAgents))
	for _, name := range commands.SupportedAgents {
		items = append(items, initAgentItem{
			name:        name,
			displayName: initAgentDisplayName(name),
			detected:    detectedSet[name],
			selected:    detectedSet[name],
		})
	}

	// If none detected, select all by default.
	anySelected := false
	for _, item := range items {
		if item.selected {
			anySelected = true
			break
		}
	}
	if !anySelected {
		for i := range items {
			items[i].selected = true
		}
	}

	return initSelectorModel{
		items:  items,
		width:  80,
		height: 24,
	}
}

func (m initSelectorModel) Init() tea.Cmd {
	return nil
}

func (m initSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.canceled = true
			return m, tea.Quit
		case tea.KeyEsc:
			m.canceled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		case tea.KeyUp, tea.KeyShiftTab:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown, tea.KeyTab:
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case tea.KeySpace:
			m.items[m.cursor].selected = !m.items[m.cursor].selected
		case tea.KeyRunes:
			if msg.String() == "a" {
				allSelected := true
				for _, item := range m.items {
					if !item.selected {
						allSelected = false
						break
					}
				}
				for i := range m.items {
					m.items[i].selected = !allSelected
				}
			}
		}
	}
	return m, nil
}

func (m initSelectorModel) View() string {
	var b strings.Builder
	b.WriteString("Praetor Init — Select agents to install\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ]"
		if item.selected {
			check = "[x]"
		}
		status := ""
		if item.detected {
			status = " (detected)"
		}
		fmt.Fprintf(&b, "%s%s %s%s\n", cursor, check, item.displayName, status)
	}

	b.WriteString("\nspace toggle  •  a toggle all  •  enter confirm  •  esc cancel")
	return b.String()
}

func initAgentDisplayName(name string) string {
	switch name {
	case "claude":
		return "Claude Code"
	case "cursor":
		return "Cursor"
	case "codex":
		return "Codex CLI"
	default:
		return name
	}
}
