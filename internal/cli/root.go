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
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(newRunCmd())
	root.AddCommand(newPlanCmd())
	root.AddCommand(newExecCmd())
	return root
}
