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
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
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
		Use:   "list",
		Short: "List available shared commands",
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

