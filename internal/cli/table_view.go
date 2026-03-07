package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type tableViewColumnSpec struct {
	Title    string
	MinWidth int
	MaxWidth int
}

type tableViewSpec struct {
	Title       string
	Summary     []string
	Columns     []tableViewColumnSpec
	Rows        []table.Row
	Empty       string
	NoColor     bool
	PlainRender func() error
}

type tableViewModel struct {
	spec   tableViewSpec
	table  table.Model
	width  int
	height int
}

func renderTableView(cmd *cobra.Command, spec tableViewSpec) error {
	if len(spec.Rows) == 0 || !isInteractiveInput(cmd.InOrStdin()) || !isInteractiveOutput(cmd.OutOrStdout()) {
		if spec.PlainRender != nil {
			return spec.PlainRender()
		}
		if strings.TrimSpace(spec.Empty) != "" {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), spec.Empty)
			return err
		}
		return nil
	}
	return runTableView(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), spec)
}

func runTableView(ctx context.Context, stdin io.Reader, out io.Writer, spec tableViewSpec) error {
	input, ok := stdin.(*os.File)
	if !ok {
		return errors.New("interactive table view requires terminal stdin")
	}
	output, ok := out.(*os.File)
	if !ok {
		return errors.New("interactive table view requires terminal stdout")
	}

	model := newTableViewModel(spec)
	_, err := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output)).Run()
	return err
}

func newTableViewModel(spec tableViewSpec) tableViewModel {
	initialWidth := 106
	columns := buildTableViewColumns(spec.Columns, spec.Rows, initialWidth)
	rows := normalizeTableViewRows(spec.Rows, len(columns))

	m := tableViewModel{
		spec:   spec,
		width:  110,
		height: 28,
	}
	m.table = table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithRows(rows),
		table.WithWidth(initialWidth),
		table.WithHeight(20),
		table.WithStyles(tableViewStyles(spec.NoColor)),
	)
	m.resize()
	return m
}

func (m tableViewModel) Init() tea.Cmd {
	return nil
}

func (m tableViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		}
		if len(msg.Runes) == 1 && (msg.Runes[0] == 'q' || msg.Runes[0] == 'Q') {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m tableViewModel) View() string {
	var b strings.Builder
	title := strings.TrimSpace(m.spec.Title)
	if title != "" {
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	for _, line := range m.spec.Summary {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(m.spec.Summary) > 0 {
		b.WriteString("\n")
	}
	if len(m.spec.Rows) == 0 {
		b.WriteString(strings.TrimSpace(m.spec.Empty))
		b.WriteString("\n")
	} else {
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}
	b.WriteString("\n↑/↓ move  •  pgup/pgdn scroll  •  home/end jump  •  q esc close")
	return b.String()
}

func (m *tableViewModel) resize() {
	width := m.width
	if width <= 0 {
		width = 110
	}
	height := m.height
	if height <= 0 {
		height = 28
	}

	tableWidth := maxInt(width-4, 60)
	tableHeight := maxInt(height-len(m.spec.Summary)-7, 8)

	m.table.SetWidth(tableWidth)
	m.table.SetHeight(tableHeight)
	m.table.SetColumns(buildTableViewColumns(m.spec.Columns, m.spec.Rows, tableWidth))
	m.table.SetRows(normalizeTableViewRows(m.spec.Rows, len(m.table.Columns())))
	m.table.UpdateViewport()
}

func buildTableViewColumns(specs []tableViewColumnSpec, rows []table.Row, totalWidth int) []table.Column {
	if len(specs) == 0 {
		return nil
	}

	available := totalWidth - len(specs)*3
	if available < len(specs)*8 {
		available = len(specs) * 8
	}

	widths := make([]int, len(specs))
	total := 0
	for i, spec := range specs {
		minWidth := spec.MinWidth
		if minWidth <= 0 {
			minWidth = len(spec.Title)
		}
		maxWidth := spec.MaxWidth
		if maxWidth < minWidth {
			maxWidth = minWidth
		}
		width := maxInt(len(spec.Title), minWidth)
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			width = maxInt(width, len(row[i]))
		}
		if width > maxWidth {
			width = maxWidth
		}
		if width < minWidth {
			width = minWidth
		}
		widths[i] = width
		total += width
	}

	for total > available {
		index := -1
		bestWidth := 0
		for i, spec := range specs {
			minWidth := spec.MinWidth
			if minWidth <= 0 {
				minWidth = len(spec.Title)
			}
			if widths[i] > minWidth && widths[i] > bestWidth {
				bestWidth = widths[i]
				index = i
			}
		}
		if index < 0 {
			break
		}
		widths[index]--
		total--
	}

	columns := make([]table.Column, 0, len(specs))
	for i, spec := range specs {
		columns = append(columns, table.Column{
			Title: spec.Title,
			Width: widths[i],
		})
	}
	return columns
}

func normalizeTableViewRows(rows []table.Row, columnCount int) []table.Row {
	if columnCount <= 0 {
		return rows
	}

	normalized := make([]table.Row, 0, len(rows))
	for _, row := range rows {
		if len(row) == columnCount {
			normalized = append(normalized, row)
			continue
		}
		padded := make(table.Row, columnCount)
		copy(padded, row)
		normalized = append(normalized, padded)
	}
	return normalized
}

func tableViewStyles(noColor bool) table.Styles {
	if noColor {
		return table.Styles{
			Header:   lipgloss.NewStyle().Bold(true).Padding(0, 1),
			Cell:     lipgloss.NewStyle().Padding(0, 1),
			Selected: lipgloss.NewStyle().Bold(true).Padding(0, 1),
		}
	}

	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("252")).
		BorderForeground(lipgloss.Color("240"))
	styles.Cell = styles.Cell.Padding(0, 1)
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("31")).
		Bold(true)
	return styles
}
