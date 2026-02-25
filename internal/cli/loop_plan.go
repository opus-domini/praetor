package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/loop"
	"github.com/spf13/cobra"
)

func newLoopPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Manage plan files and execution state",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newLoopPlanNewCmd())
	cmd.AddCommand(newLoopPlanStatusCmd())
	cmd.AddCommand(newLoopPlanListCmd())
	cmd.AddCommand(newLoopPlanResetCmd())
	return cmd
}

func newLoopPlanNewCmd() *cobra.Command {
	var baseDir string

	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Create a new plan skeleton in docs/plans",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := strings.TrimSpace(baseDir)
			if root == "" {
				root = "."
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("resolve base directory: %w", err)
			}

			path, err := loop.NewPlanFile(args[0], time.Now(), absRoot)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", path)
			return err
		},
	}

	cmd.Flags().StringVar(&baseDir, "base-dir", ".", "Repository root where docs/plans is located")
	return cmd
}

func newLoopPlanStatusCmd() *cobra.Command {
	var planFile string
	var stateRoot string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show execution status for one plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			planFile = strings.TrimSpace(planFile)
			if planFile == "" {
				return errors.New("--plan is required")
			}
			absPlan, err := filepath.Abs(planFile)
			if err != nil {
				return fmt.Errorf("resolve plan path: %w", err)
			}
			resolvedStateRoot, err := resolveStateRoot(stateRoot, filepath.Dir(absPlan))
			if err != nil {
				return err
			}

			store := loop.NewStore(resolvedStateRoot)
			status, err := store.Status(absPlan)
			if err != nil {
				return err
			}
			return printPlanStatus(cmd, status)
		},
	}

	cmd.Flags().StringVar(&planFile, "plan", "", "Plan JSON file path (required)")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "Runtime state root directory (default: ~/.praetor/projects/<project-hash>)")
	return cmd
}

func newLoopPlanListCmd() *cobra.Command {
	var stateRoot string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List plans with execution state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedStateRoot, err := resolveStateRoot(stateRoot, ".")
			if err != nil {
				return err
			}

			store := loop.NewStore(resolvedStateRoot)
			statuses, err := store.ListPlanStatuses()
			if err != nil {
				return err
			}
			if len(statuses) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "No execution state found in %s\n", store.StateDir())
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-55s %5s %5s %5s  %s\n", "Plan", "Done", "Open", "Total", "Status"); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-55s %5s %5s %5s  %s\n", "----", "----", "----", "-----", "------"); err != nil {
				return err
			}

			for _, status := range statuses {
				label := "in_progress"
				if status.Open == 0 {
					label = "completed"
				}
				if status.Running {
					label = "running"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-55s %5d %5d %5d  %s\n", status.PlanFile, status.Done, status.Open, status.Total, label); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&stateRoot, "state-root", "", "Runtime state root directory (default: ~/.praetor/projects/<project-hash>)")
	return cmd
}

func newLoopPlanResetCmd() *cobra.Command {
	var planFile string
	var stateRoot string

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Clear execution state for one plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			planFile = strings.TrimSpace(planFile)
			if planFile == "" {
				return errors.New("--plan is required")
			}
			absPlan, err := filepath.Abs(planFile)
			if err != nil {
				return fmt.Errorf("resolve plan path: %w", err)
			}

			plan, err := loop.LoadPlan(absPlan)
			if err != nil {
				return err
			}

			resolvedStateRoot, err := resolveStateRoot(stateRoot, filepath.Dir(absPlan))
			if err != nil {
				return err
			}

			store := loop.NewStore(resolvedStateRoot)
			removed, err := store.ResetPlanRuntime(absPlan, plan)
			if err != nil {
				return err
			}
			if removed == 0 {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Nothing to reset for: %s\n", absPlan)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Reset complete. Removed %d file(s).\n", removed)
			return err
		},
	}

	cmd.Flags().StringVar(&planFile, "plan", "", "Plan JSON file path (required)")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "Runtime state root directory (default: ~/.praetor/projects/<project-hash>)")
	return cmd
}
