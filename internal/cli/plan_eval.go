package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
		Args:    cobra.ExactArgs(1),
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
			report, err := pipeline.EvaluatePlanFlow(store, args[0], strings.TrimSpace(runID))
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
}
