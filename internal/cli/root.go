package cli

import (
	"fmt"

	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the praetor CLI root command.
func NewRootCmd() *cobra.Command {
	homePath := "~/.config/praetor"
	if resolved, err := localstate.DefaultHome(); err == nil {
		homePath = resolved
	}

	root := &cobra.Command{
		Use:   "praetor",
		Short: "Lead. Delegate. Dominate.",
		Long: fmt.Sprintf(`Praetor orchestrates AI agents through a single command surface.

Home: %s  (override: $PRAETOR_HOME)`, homePath),
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
