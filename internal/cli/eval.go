package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/opus-domini/praetor/internal/orchestration/pipeline"
	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	var format string
	var window string
	var failOnFail bool

	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluate local project execution quality",
		Long: `Evaluate the latest local runs across plans in the current project.
This command aggregates acceptance quality, gate integrity, parse failures,
retries, stalls, and cost from local runtime artifacts.`,
		Example: `  praetor eval
  praetor eval --window 168h
  praetor eval --format json --fail-on-fail`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			format = strings.ToLower(strings.TrimSpace(format))
			switch format {
			case "table", "json":
			default:
				return fmt.Errorf("unsupported format %q (allowed: table, json)", format)
			}

			window = strings.TrimSpace(window)
			windowDuration := 7 * 24 * time.Hour
			if window != "" {
				parsed, err := time.ParseDuration(window)
				if err != nil {
					return fmt.Errorf("invalid --window duration: %w", err)
				}
				windowDuration = parsed
			}

			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			report, err := pipeline.EvaluateProjectFlow(store, windowDuration)
			if err != nil {
				return err
			}

			if format == "json" {
				encoded, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("encode project eval report: %w", err)
				}
				encoded = append(encoded, '\n')
				if _, err := cmd.OutOrStdout().Write(encoded); err != nil {
					return err
				}
			} else {
				if err := printProjectEvalTable(cmd, report); err != nil {
					return err
				}
			}

			if failOnFail && strings.EqualFold(strings.TrimSpace(report.Summary.Verdict), "fail") {
				return newExitError(1, errors.New("project eval failed quality checks"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&window, "window", "168h", "Time window for latest plan runs (0 to disable filtering)")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table or json")
	cmd.Flags().BoolVar(&failOnFail, "fail-on-fail", false, "Return non-zero exit code when project verdict is fail")
	return cmd
}

func printProjectEvalTable(cmd *cobra.Command, report pipeline.ProjectEvalReport) error {
	rows := make([]table.Row, 0, len(report.Plans))
	for _, item := range report.Plans {
		rows = append(rows, table.Row{
			item.PlanSlug,
			strings.ToUpper(strings.TrimSpace(item.Verdict)),
			fmt.Sprintf("%.2f", item.AcceptanceRate),
			fmt.Sprintf("%d", item.Summary.FailedCount),
			fmt.Sprintf("%d", item.Summary.StalledTasks),
			fmt.Sprintf("%d", item.Summary.ParseErrorTasks),
			fmt.Sprintf("%.2f", item.Summary.AvgRetriesPerTask),
			fmt.Sprintf("%.4f", item.Summary.TotalCostUSD),
			item.RunID,
		})
	}

	summary := []string{
		fmt.Sprintf("Project home: %s", report.ProjectHome),
		fmt.Sprintf("Generated: %s", report.GeneratedAt),
		fmt.Sprintf("Verdict: %s", strings.ToUpper(strings.TrimSpace(report.Summary.Verdict))),
		fmt.Sprintf("Plans: %d (pass=%d warn=%d fail=%d)", report.Summary.PlanCount, report.Summary.PassCount, report.Summary.WarnCount, report.Summary.FailCount),
		fmt.Sprintf("Task acceptance: %.2f (%d/%d)", report.Summary.AcceptanceRate, report.Summary.AcceptedTaskCount, report.Summary.TaskCount),
		fmt.Sprintf("Avg retries/task: %.2f  Total cost: $%.4f  Avg cost/plan: $%.4f", report.Summary.AvgRetriesPerTask, report.Summary.TotalCostUSD, report.Summary.AvgCostUSDPerPlan),
	}
	if strings.TrimSpace(report.Window) != "" {
		summary = append(summary[:1], append([]string{fmt.Sprintf("Window: %s", report.Window)}, summary[1:]...)...)
	}
	if len(report.Summary.Reasons) > 0 {
		summary = append(summary, "Reasons: "+strings.Join(report.Summary.Reasons, " | "))
	}

	return renderTableView(cmd, tableViewSpec{
		Title:   "Project Evaluation",
		Summary: summary,
		Columns: []tableViewColumnSpec{
			{Title: "Plan", MinWidth: 16, MaxWidth: 24},
			{Title: "Verdict", MinWidth: 8, MaxWidth: 8},
			{Title: "Accept", MinWidth: 8, MaxWidth: 10},
			{Title: "Failed", MinWidth: 8, MaxWidth: 8},
			{Title: "Stalls", MinWidth: 8, MaxWidth: 8},
			{Title: "ParseErr", MinWidth: 8, MaxWidth: 9},
			{Title: "Retries", MinWidth: 8, MaxWidth: 8},
			{Title: "CostUSD", MinWidth: 9, MaxWidth: 10},
			{Title: "RunID", MinWidth: 12, MaxWidth: 20},
		},
		Rows: rows,
		PlainRender: func() error {
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Project home: %s\n", report.ProjectHome); err != nil {
				return err
			}
			if strings.TrimSpace(report.Window) != "" {
				if _, err := fmt.Fprintf(out, "Window: %s\n", report.Window); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(out, "Generated: %s\n", report.GeneratedAt); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Verdict: %s\n", strings.ToUpper(strings.TrimSpace(report.Summary.Verdict))); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Plans: %d (pass=%d warn=%d fail=%d)\n",
				report.Summary.PlanCount,
				report.Summary.PassCount,
				report.Summary.WarnCount,
				report.Summary.FailCount,
			); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Task acceptance: %.2f (%d/%d)\n",
				report.Summary.AcceptanceRate,
				report.Summary.AcceptedTaskCount,
				report.Summary.TaskCount,
			); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Avg retries/task: %.2f  Total cost: $%.4f  Avg cost/plan: $%.4f\n",
				report.Summary.AvgRetriesPerTask,
				report.Summary.TotalCostUSD,
				report.Summary.AvgCostUSDPerPlan,
			); err != nil {
				return err
			}
			if len(report.Summary.Reasons) > 0 {
				if _, err := fmt.Fprintln(out, "Reasons:"); err != nil {
					return err
				}
				for _, reason := range report.Summary.Reasons {
					if _, err := fmt.Fprintf(out, "- %s\n", strings.TrimSpace(reason)); err != nil {
						return err
					}
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "%-20s %-8s %-10s %-10s %-8s %-10s %-10s %-10s %s\n",
				"Plan", "Verdict", "Acceptance", "Failed", "Stalls", "ParseErr", "Retries", "CostUSD", "RunID"); err != nil {
				return err
			}
			for _, item := range report.Plans {
				if _, err := fmt.Fprintf(out, "%-20s %-8s %-10.2f %-10d %-8d %-10d %-10.2f %-10.4f %s\n",
					item.PlanSlug,
					strings.ToUpper(strings.TrimSpace(item.Verdict)),
					item.AcceptanceRate,
					item.Summary.FailedCount,
					item.Summary.StalledTasks,
					item.Summary.ParseErrorTasks,
					item.Summary.AvgRetriesPerTask,
					item.Summary.TotalCostUSD,
					item.RunID,
				); err != nil {
					return err
				}
			}
			return nil
		},
	})
}
