package cli

import (
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/loop"
	"github.com/spf13/cobra"
)

func newLoopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loop",
		Short: "Run plan-driven agent orchestration loops",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newLoopRunCmd())
	cmd.AddCommand(newLoopPlanCmd())
	return cmd
}

func resolveStateRoot(explicitRoot, projectDir string) (string, error) {
	root, err := loop.ResolveStateRoot(explicitRoot, projectDir)
	if err != nil {
		return "", err
	}
	return root, nil
}

func printPlanStatus(cmd *cobra.Command, status loop.PlanStatus) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan:     %s\n", status.PlanFile); err != nil {
		return err
	}
	if status.StateFile == "" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "State:    not started"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Tasks:    %d (all pending)\n", status.Total); err != nil {
			return err
		}
		return nil
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State:    %s\n", status.StateFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Updated:  %s\n", fallback(status.UpdatedAt, "-")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Progress: %d/%d tasks done\n", status.Done, status.Total); err != nil {
		return err
	}
	stateLabel := "in progress"
	if status.Open == 0 {
		stateLabel = "completed"
	}
	if status.Running {
		stateLabel = "running"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Status:   %s\n", stateLabel); err != nil {
		return err
	}

	if len(status.Tasks) > 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), ""); err != nil {
			return err
		}
		for _, task := range status.Tasks {
			mark := " "
			if task.Status == loop.TaskStatusDone {
				mark = "x"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", mark, task.ID, task.Title); err != nil {
				return err
			}
		}
	}
	return nil
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
