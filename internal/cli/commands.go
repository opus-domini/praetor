package cli

import (
	"fmt"

	"github.com/opus-domini/praetor/internal/commands"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newCommandsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commands",
		Short: "Manage shared agent commands",
		Long: `Generate shared agent commands (.md files with tool whitelists) and create
symlinks so Claude Code, Cursor, and Codex can discover them from a single source.

Built-in commands: plan-create, plan-run, review-task, doctor, diagnose.
Commands are stored in .agents/commands/ with symlinks from each agent directory.`,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newCommandsSyncCmd())
	cmd.AddCommand(newCommandsListCmd())
	return cmd
}

func newCommandsSyncCmd() *cobra.Command {
	var agents []string
	var noColor bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Generate shared agent commands and create symlinks",
		Example: `  # Sync for all supported agents (claude, cursor, codex)
  praetor commands sync

  # Sync for specific agents only
  praetor commands sync --agents claude,cursor`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			projectRoot, err := workspace.ResolveProjectRoot("")
			if err != nil {
				return fmt.Errorf("resolve project root: %w", err)
			}

			if err := commands.Sync(projectRoot, agents); err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), noColor)
			r.Success("Commands synced to .agents/commands/")

			active := agents
			if len(active) == 0 {
				active = commands.SupportedAgents
			}
			for _, agent := range active {
				r.Info(fmt.Sprintf(".%s/commands/ -> .agents/commands/", agent))
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&agents, "agents", commands.SupportedAgents, "Agent directories to create symlinks for")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}

func newCommandsListCmd() *cobra.Command {
	var noColor bool

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available shared commands",
		Example: "  praetor commands list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			projectRoot, err := workspace.ResolveProjectRoot("")
			if err != nil {
				return fmt.Errorf("resolve project root: %w", err)
			}

			names, err := commands.List(projectRoot)
			if err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), noColor)

			if len(names) == 0 {
				r.Info("No commands found. Run 'praetor commands sync' to generate them.")
				return nil
			}

			r.Header("Shared Agent Commands")
			for _, name := range names {
				r.Info(name)
			}
			r.Dim(fmt.Sprintf("\n  %d command(s) in .agents/commands/", len(names)))
			return nil
		},
	}

	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}
