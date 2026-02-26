package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Create, manage, and execute task plans",
		Long: `Create, manage, and execute task plans.

Plans are JSON files that define a sequence of tasks with dependencies,
executors, and reviewers. Use "praetor plan run <plan>" to execute a plan.`,
	}

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newPlanCreateCmd())
	cmd.AddCommand(newPlanListCmd())
	cmd.AddCommand(newPlanStatusCmd())
	cmd.AddCommand(newPlanResetCmd())
	cmd.AddCommand(newPlanResumeCmd())
	return cmd
}

func newPlanCreateCmd() *cobra.Command {
	var baseDir string

	cmd := &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a new plan from a template",
		Long:  `Create a new plan skeleton in docs/plans/ with two sample tasks.`,
		Example: `  praetor plan create my-feature
  praetor plan create auth-refactor --base-dir /path/to/repo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			root := strings.TrimSpace(baseDir)
			if root == "" {
				root = "."
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("resolve base directory: %w", err)
			}

			path, err := domain.NewPlanFile(args[0], time.Now(), absRoot)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", path)
			return err
		},
	}

	cmd.Flags().StringVar(&baseDir, "base-dir", ".", "Repository root where docs/plans/ is located")
	return cmd
}

func newPlanStatusCmd() *cobra.Command {
	var stateRoot string

	cmd := &cobra.Command{
		Use:     "status <plan-file>",
		Short:   "Show execution status for a plan",
		Example: `  praetor plan status docs/plans/my-plan.json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			absPlan, err := filepath.Abs(strings.TrimSpace(args[0]))
			if err != nil {
				return fmt.Errorf("resolve plan path: %w", err)
			}

			resolvedStateRoot, err := resolveStateRoot(stateRoot, filepath.Dir(absPlan))
			if err != nil {
				return err
			}

			store := localstate.NewStore(resolvedStateRoot, "")
			status, err := store.Status(absPlan)
			if err != nil {
				return err
			}
			return printPlanStatus(cmd, status)
		},
	}

	cmd.Flags().StringVar(&stateRoot, "state-root", "", "State root directory (default: $XDG_STATE_HOME/praetor/projects/<hash>)")
	return cmd
}

func newPlanListCmd() *cobra.Command {
	var stateRoot string

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List tracked plans for current project",
		Example: `  praetor plan list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			resolvedStateRoot, err := resolveStateRoot(stateRoot, ".")
			if err != nil {
				return err
			}

			store := localstate.NewStore(resolvedStateRoot, "")
			statuses, err := store.ListPlanStatuses()
			if err != nil {
				return err
			}
			if len(statuses) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "No plans tracked for current project in %s\n", store.StateDir())
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-55s %5s %5s %5s %5s  %s\n", "Plan", "Done", "Fail", "Left", "Total", "Status"); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-55s %5s %5s %5s %5s  %s\n", "----", "----", "----", "----", "-----", "------"); err != nil {
				return err
			}

			for _, status := range statuses {
				label := "in_progress"
				if status.Active == 0 && status.Failed == 0 {
					label = "completed"
				} else if status.Active == 0 && status.Failed > 0 {
					label = "failed"
				}
				if status.Running {
					label = "running"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-55s %5d %5d %5d %5d  %s\n", status.PlanFile, status.Done, status.Failed, status.Active, status.Total, label); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&stateRoot, "state-root", "", "State root directory (default: $XDG_STATE_HOME/praetor/projects/<hash>)")
	return cmd
}

func newPlanResetCmd() *cobra.Command {
	var stateRoot string
	var force bool

	cmd := &cobra.Command{
		Use:     "reset <plan-file>",
		Short:   "Clear execution state for a plan",
		Example: `  praetor plan reset docs/plans/my-plan.json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			absPlan, err := filepath.Abs(strings.TrimSpace(args[0]))
			if err != nil {
				return fmt.Errorf("resolve plan path: %w", err)
			}

			plan, err := domain.LoadPlan(absPlan)
			if err != nil {
				return err
			}

			resolvedStateRoot, err := resolveStateRoot(stateRoot, filepath.Dir(absPlan))
			if err != nil {
				return err
			}

			store := localstate.NewStore(resolvedStateRoot, "")
			running, pid := store.IsPlanRunning(absPlan)
			if running && !force {
				return fmt.Errorf("plan is currently running (pid=%d); use --force to reset anyway", pid)
			}
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

	cmd.Flags().StringVar(&stateRoot, "state-root", "", "State root directory (default: $XDG_STATE_HOME/praetor/projects/<hash>)")
	cmd.Flags().BoolVar(&force, "force", false, "Force reset even if a running lock exists")
	return cmd
}

func newPlanResumeCmd() *cobra.Command {
	var stateRoot string

	cmd := &cobra.Command{
		Use:     "resume <plan-file>",
		Short:   "Restore the latest local snapshot state for a plan",
		Example: `  praetor plan resume docs/plans/my-plan.json`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			absPlan, err := filepath.Abs(strings.TrimSpace(args[0]))
			if err != nil {
				return fmt.Errorf("resolve plan path: %w", err)
			}

			projectRoot, err := workspace.ResolveProjectRoot(filepath.Dir(absPlan))
			if err != nil {
				return err
			}
			snapshot, snapshotPath, err := localstate.LoadLatestLocalSnapshot(projectRoot, absPlan)
			if err != nil {
				return err
			}
			if strings.TrimSpace(snapshotPath) == "" {
				return fmt.Errorf("no local snapshot found for plan: %s", absPlan)
			}

			resolvedStateRoot, err := resolveStateRoot(stateRoot, projectRoot)
			if err != nil {
				return err
			}
			store := localstate.NewStore(resolvedStateRoot, "")
			if err := store.Init(); err != nil {
				return err
			}
			if err := store.WriteState(absPlan, snapshot.State); err != nil {
				return fmt.Errorf("persist resumed state: %w", err)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Resumed from: %s\n", snapshotPath); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State updated: %s\n", store.StateFile(absPlan)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Progress: %d/%d done\n", snapshot.State.DoneCount(), len(snapshot.State.Tasks))
			return err
		},
	}

	cmd.Flags().StringVar(&stateRoot, "state-root", "", "State root directory (default: $XDG_STATE_HOME/praetor/projects/<hash>)")
	return cmd
}

func resolveStateRoot(explicitRoot, projectDir string) (string, error) {
	root, err := localstate.ResolveStateRoot(explicitRoot, projectDir)
	if err != nil {
		return "", err
	}
	return root, nil
}

func printPlanStatus(cmd *cobra.Command, status domain.PlanStatus) error {
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
	if status.Failed > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Failed:   %d\n", status.Failed); err != nil {
			return err
		}
	}
	stateLabel := "in progress"
	if status.Active == 0 && status.Failed == 0 {
		stateLabel = "completed"
	} else if status.Active == 0 && status.Failed > 0 {
		stateLabel = "failed"
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
			mark := taskStatusMark(task.Status)
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", mark, task.ID, task.Title); err != nil {
				return err
			}
		}
	}
	return nil
}

func taskStatusMark(status domain.TaskStatus) string {
	switch status {
	case domain.TaskDone:
		return "x"
	case domain.TaskFailed:
		return "!"
	case domain.TaskExecuting, domain.TaskReviewing:
		return ">"
	default:
		return " "
	}
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
