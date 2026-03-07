package cli

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/spf13/cobra"
)

func TestRenderTableViewFallsBackToPlainRenderer(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&stdout)

	called := false
	err := renderTableView(cmd, tableViewSpec{
		Title: "Example",
		Columns: []tableViewColumnSpec{
			{Title: "Name", MinWidth: 4, MaxWidth: 12},
		},
		Rows: []table.Row{{"alpha"}},
		PlainRender: func() error {
			called = true
			_, err := stdout.WriteString("plain output\n")
			return err
		},
	})
	if err != nil {
		t.Fatalf("renderTableView returned error: %v", err)
	}
	if !called {
		t.Fatal("expected plain renderer to be called")
	}
	if got := stdout.String(); got != "plain output\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestBuildTableViewColumnsRespectsWidthBounds(t *testing.T) {
	t.Parallel()

	columns := buildTableViewColumns([]tableViewColumnSpec{
		{Title: "Plan", MinWidth: 6, MaxWidth: 12},
		{Title: "Status", MinWidth: 8, MaxWidth: 10},
		{Title: "Notes", MinWidth: 10, MaxWidth: 14},
	}, []table.Row{
		{"my-feature", "completed", "very long note that should be clipped"},
	}, 36)

	if len(columns) != 3 {
		t.Fatalf("len(columns) = %d, want 3", len(columns))
	}
	if columns[0].Width < 6 || columns[0].Width > 12 {
		t.Fatalf("plan width = %d", columns[0].Width)
	}
	if columns[1].Width < 8 || columns[1].Width > 10 {
		t.Fatalf("status width = %d", columns[1].Width)
	}
	if columns[2].Width < 10 || columns[2].Width > 14 {
		t.Fatalf("notes width = %d", columns[2].Width)
	}
	total := 0
	for _, col := range columns {
		total += col.Width
	}
	if total > 36 {
		t.Fatalf("total column width = %d, want <= 36", total)
	}
}

func TestNewTableViewModelInitializesColumnsBeforeRows(t *testing.T) {
	t.Parallel()

	model := newTableViewModel(tableViewSpec{
		Title: "Example",
		Columns: []tableViewColumnSpec{
			{Title: "Task", MinWidth: 6, MaxWidth: 12},
			{Title: "Status", MinWidth: 8, MaxWidth: 12},
		},
		Rows: []table.Row{
			{"TASK-001", "done"},
		},
	})

	if got := len(model.table.Columns()); got != 2 {
		t.Fatalf("len(columns) = %d, want 2", got)
	}
	if got := len(model.table.Rows()); got != 1 {
		t.Fatalf("len(rows) = %d, want 1", got)
	}
}

func TestNormalizeTableViewRowsPadsShortRows(t *testing.T) {
	t.Parallel()

	rows := normalizeTableViewRows([]table.Row{
		{"alpha"},
	}, 3)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if len(rows[0]) != 3 {
		t.Fatalf("len(row) = %d, want 3", len(rows[0]))
	}
	if rows[0][0] != "alpha" || rows[0][1] != "" || rows[0][2] != "" {
		t.Fatalf("unexpected padded row: %#v", rows[0])
	}
}
