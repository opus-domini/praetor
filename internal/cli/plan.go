package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Create, manage, and execute task plans",
		Long: `Create, manage, and execute task plans.

Plans are JSON files identified by slug and stored under the praetor home directory.
Use "praetor plan run <slug>" to execute a plan.`,
	}

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newPlanCreateCmd())
	cmd.AddCommand(newPlanEditCmd())
	cmd.AddCommand(newPlanListCmd())
	cmd.AddCommand(newPlanStatusCmd())
	cmd.AddCommand(newPlanShowCmd())
	cmd.AddCommand(newPlanResetCmd())
	cmd.AddCommand(newPlanResumeCmd())
	cmd.AddCommand(newPlanPathCmd())
	return cmd
}

func newPlanCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a new plan from a template",
		Long:  `Create a new plan skeleton in the project plans directory.`,
		Example: `  praetor plan create my-feature
  praetor plan create auth-refactor`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			if err := store.Init(); err != nil {
				return err
			}

			path, err := domain.NewPlanFile(args[0], store.PlansDir())
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", path); err != nil {
				return err
			}
			return openEditor(path)
		},
	}

	return cmd
}

func newPlanEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "edit <slug>",
		Short:   "Open a plan in $EDITOR",
		Example: `  praetor plan edit my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			path := store.PlanFile(args[0])
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("plan not found: %s", args[0])
			}
			return openEditor(path)
		},
	}
	return cmd
}

func newPlanShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <slug>",
		Short:   "Print plan JSON to stdout",
		Example: `  praetor plan show my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			path := store.PlanFile(args[0])
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("plan not found: %s", args[0])
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), string(data))
			return err
		},
	}
	return cmd
}

func newPlanPathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "path <slug>",
		Short:   "Print absolute path of a plan file",
		Example: `  praetor plan path my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), store.PlanFile(args[0]))
			return err
		},
	}
	return cmd
}

func newPlanStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status <slug>",
		Short:   "Show execution status for a plan",
		Example: `  praetor plan status my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			status, err := store.Status(args[0])
			if err != nil {
				return err
			}
			return printPlanStatus(cmd, status)
		},
	}

	return cmd
}

func newPlanListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List tracked plans for current project",
		Example: `  praetor plan list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			statuses, err := store.ListPlanStatuses()
			if err != nil {
				return err
			}
			if len(statuses) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "No plans found in %s\n", store.PlansDir())
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-30s %5s %5s %5s %5s  %s\n", "Plan", "Done", "Fail", "Left", "Total", "Status"); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-30s %5s %5s %5s %5s  %s\n", "----", "----", "----", "----", "-----", "------"); err != nil {
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
				if status.StateFile == "" {
					label = "not_started"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-30s %5d %5d %5d %5d  %s\n", status.PlanSlug, status.Done, status.Failed, status.Active, status.Total, label); err != nil {
					return err
				}
			}
			return nil
		},
	}

	return cmd
}

func newPlanResetCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:     "reset <slug>",
		Short:   "Clear execution state for a plan",
		Example: `  praetor plan reset my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			slug := args[0]
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			plan, err := domain.LoadPlan(store.PlanFile(slug))
			if err != nil {
				return err
			}

			running, pid := store.IsPlanRunning(slug)
			if running && !force {
				return fmt.Errorf("plan is currently running (pid=%d); use --force to reset anyway", pid)
			}
			removed, err := store.ResetPlanRuntime(slug, plan)
			if err != nil {
				return err
			}
			if removed == 0 {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Nothing to reset for: %s\n", slug)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Reset complete. Removed %d file(s).\n", removed)
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force reset even if a running lock exists")
	return cmd
}

func newPlanResumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resume <slug>",
		Short:   "Restore the latest local snapshot state for a plan",
		Example: `  praetor plan resume my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			slug := args[0]
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			snapshot, snapshotPath, err := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), slug)
			if err != nil {
				return err
			}
			if strings.TrimSpace(snapshotPath) == "" {
				return fmt.Errorf("no local snapshot found for plan: %s", slug)
			}

			if err := store.Init(); err != nil {
				return err
			}
			if err := store.WriteState(slug, snapshot.State); err != nil {
				return fmt.Errorf("persist resumed state: %w", err)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Resumed from: %s\n", snapshotPath); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State updated: %s\n", store.StateFile(slug)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Progress: %d/%d done\n", snapshot.State.DoneCount(), len(snapshot.State.Tasks))
			return err
		},
	}

	return cmd
}

func resolveStore(projectDir string) (*localstate.Store, error) {
	root, err := localstate.ResolveProjectHome("", projectDir)
	if err != nil {
		return nil, err
	}
	return localstate.NewStore(root), nil
}

func openEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return nil
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printPlanStatus(cmd *cobra.Command, status domain.PlanStatus) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan:     %s\n", status.PlanSlug); err != nil {
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
