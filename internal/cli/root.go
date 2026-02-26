package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the praetor CLI root command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "praetor",
		Short: "Lead. Delegate. Dominate.",
		Long: `Praetor orchestrates AI agents through a single command surface.
It drives Claude Code and Codex as subprocess agents, coordinated by an
executor/reviewer pipeline with worktree isolation, cost tracking, and crash recovery.`,
		// Print help instead of a cryptic "unknown command" error.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(newPlanCmd())
	root.AddCommand(newExecCmd())
	return root
}
