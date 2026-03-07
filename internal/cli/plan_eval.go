package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/opus-domini/praetor/internal/orchestration/pipeline"
	"github.com/spf13/cobra"
)

func newPlanEvalCmd() *cobra.Command {
	var runID string
	var format string
	var failOnFail bool

	cmd := &cobra.Command{
		Use:     "eval <slug>",
		Short:   "Evaluate execution quality for one local plan run",
		Example: "  praetor plan eval my-plan\n  praetor plan eval my-plan --run-id run-123 --format json",
		Args:    planSlugArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			format = strings.ToLower(strings.TrimSpace(format))
			switch format {
			case "table", "json":
			default:
				return fmt.Errorf("unsupported format %q (allowed: table, json)", format)
			}

			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			slug, err := resolvePlanSlug(cmd, store, args, "evaluate")
			if err != nil {
				return err
			}
			report, err := pipeline.EvaluatePlanFlow(store, slug, strings.TrimSpace(runID))
			if err != nil {
				return err
			}

			if format == "json" {
				encoded, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return fmt.Errorf("encode plan eval report: %w", err)
				}
				encoded = append(encoded, '\n')
				if _, err := cmd.OutOrStdout().Write(encoded); err != nil {
					return err
				}
			} else {
				if err := printPlanEvalTable(cmd, report); err != nil {
					return err
				}
			}

			if failOnFail && strings.EqualFold(strings.TrimSpace(report.Summary.Verdict), "fail") {
				return newExitError(1, errors.New("plan eval failed quality checks"))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "Evaluate a specific local runtime run id (default: latest)")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table or json")
	cmd.Flags().BoolVar(&failOnFail, "fail-on-fail", false, "Return non-zero exit code when verdict is fail")
	return cmd
}

func printPlanEvalTable(cmd *cobra.Command, report pipeline.PlanEvalReport) error {
	rows := make([]table.Row, 0, len(report.Tasks))
	for _, task := range report.Tasks {
		rows = append(rows, table.Row{
			task.TaskID,
			task.Status,
			fmt.Sprintf("%t", task.Accepted),
			fmt.Sprintf("%d", task.Attempts),
			fmt.Sprintf("%d", task.RequiredGateFailures),
			fmt.Sprintf("%d", task.RequiredGateMissing),
			fmt.Sprintf("%d", task.ParseErrorCount),
			fmt.Sprintf("%d", task.StallCount),
			fmt.Sprintf("%.4f", task.CostUSD),
			strings.Join(task.FailureReasons, "; "),
		})
	}

	summary := []string{
		fmt.Sprintf("Plan: %s", report.PlanSlug),
		fmt.Sprintf("Run: %s", report.RunID),
		fmt.Sprintf("Verdict: %s", strings.ToUpper(strings.TrimSpace(report.Summary.Verdict))),
		fmt.Sprintf("Acceptance: %.2f (%d/%d)", report.Summary.AcceptanceRate, report.Summary.AcceptedCount, report.Summary.TaskCount),
		fmt.Sprintf("Failed=%d  GateFail=%d  GateMiss=%d  Parse=%d  Stalls=%d", report.Summary.FailedCount, report.Summary.RequiredGateFailureTasks, report.Summary.RequiredGateMissingTasks, report.Summary.ParseErrorTasks, report.Summary.StalledTasks),
		fmt.Sprintf("Avg retries/task=%.2f  Avg cost/task=$%.4f  Total cost=$%.4f  P95 duration=%.1fs", report.Summary.AvgRetriesPerTask, report.Summary.AvgCostUSDPerTask, report.Summary.TotalCostUSD, report.Summary.P95TaskDurationS),
	}
	if strings.TrimSpace(report.PlanName) != "" {
		summary = append(summary[:1], append([]string{fmt.Sprintf("Name: %s", report.PlanName)}, summary[1:]...)...)
	}
	if strings.TrimSpace(report.Timestamp) != "" {
		summary = append(summary, fmt.Sprintf("Timestamp: %s", report.Timestamp))
	}
	if strings.TrimSpace(report.Outcome) != "" {
		summary = append(summary, fmt.Sprintf("Outcome: %s", report.Outcome))
	}
	if len(report.Summary.Reasons) > 0 {
		summary = append(summary, "Reasons: "+strings.Join(report.Summary.Reasons, " | "))
	}
	if len(report.MissingArtifacts) > 0 {
		summary = append(summary, "Missing artifacts: "+strings.Join(report.MissingArtifacts, ", "))
	}

	return renderTableView(cmd, tableViewSpec{
		Title:   "Plan Evaluation",
		Summary: summary,
		Columns: []tableViewColumnSpec{
			{Title: "Task", MinWidth: 10, MaxWidth: 14},
			{Title: "Status", MinWidth: 10, MaxWidth: 12},
			{Title: "Accepted", MinWidth: 9, MaxWidth: 9},
			{Title: "Retries", MinWidth: 7, MaxWidth: 8},
			{Title: "GateFail", MinWidth: 8, MaxWidth: 8},
			{Title: "GateMiss", MinWidth: 8, MaxWidth: 8},
			{Title: "Parse", MinWidth: 6, MaxWidth: 6},
			{Title: "Stalls", MinWidth: 6, MaxWidth: 6},
			{Title: "CostUSD", MinWidth: 10, MaxWidth: 10},
			{Title: "Notes", MinWidth: 18, MaxWidth: 42},
		},
		Rows: rows,
		PlainRender: func() error {
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Plan: %s\n", report.PlanSlug); err != nil {
				return err
			}
			if strings.TrimSpace(report.PlanName) != "" {
				if _, err := fmt.Fprintf(out, "Name: %s\n", report.PlanName); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(out, "Run: %s\n", report.RunID); err != nil {
				return err
			}
			if strings.TrimSpace(report.Timestamp) != "" {
				if _, err := fmt.Fprintf(out, "Timestamp: %s\n", report.Timestamp); err != nil {
					return err
				}
			}
			if strings.TrimSpace(report.Outcome) != "" {
				if _, err := fmt.Fprintf(out, "Outcome: %s\n", report.Outcome); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(out, "Verdict: %s\n", strings.ToUpper(strings.TrimSpace(report.Summary.Verdict))); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Acceptance: %.2f (%d/%d)\n", report.Summary.AcceptanceRate, report.Summary.AcceptedCount, report.Summary.TaskCount); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Failed tasks: %d  Gate failures: %d  Gate missing: %d  Parse errors: %d  Stalls: %d\n",
				report.Summary.FailedCount,
				report.Summary.RequiredGateFailureTasks,
				report.Summary.RequiredGateMissingTasks,
				report.Summary.ParseErrorTasks,
				report.Summary.StalledTasks,
			); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "Avg retries/task: %.2f  Avg cost/task: $%.4f  Total cost: $%.4f  P95 task duration: %.1fs\n",
				report.Summary.AvgRetriesPerTask,
				report.Summary.AvgCostUSDPerTask,
				report.Summary.TotalCostUSD,
				report.Summary.P95TaskDurationS,
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
			if len(report.MissingArtifacts) > 0 {
				if _, err := fmt.Fprintf(out, "Missing artifacts: %s\n", strings.Join(report.MissingArtifacts, ", ")); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "%-12s %-10s %-9s %-8s %-10s %-10s %-8s %-10s %-10s %s\n",
				"Task", "Status", "Accepted", "Retries", "GateFail", "GateMiss", "Parse", "Stalls", "CostUSD", "Notes"); err != nil {
				return err
			}
			for _, task := range report.Tasks {
				notes := strings.Join(task.FailureReasons, "; ")
				if _, err := fmt.Fprintf(out, "%-12s %-10s %-9t %-8d %-10d %-10d %-8d %-10d %-10.4f %s\n",
					task.TaskID,
					task.Status,
					task.Accepted,
					task.Attempts,
					task.RequiredGateFailures,
					task.RequiredGateMissing,
					task.ParseErrorCount,
					task.StallCount,
					task.CostUSD,
					notes,
				); err != nil {
					return err
				}
			}
			return nil
		},
	})
}
