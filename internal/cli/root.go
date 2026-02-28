package cli

import (
	"fmt"

	"github.com/opus-domini/praetor/internal/cli/branding"
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
		Long:  fmt.Sprintf("%s\nPraetor orchestrates AI agents through a single command surface.\n\nHome: %s  (override: $PRAETOR_HOME)", branding.ASCIILogo, homePath),
		// Print help instead of a cryptic "unknown command" error.
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(newPlanCmd())
	root.AddCommand(newExecCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newFmtCmd())
	root.AddCommand(newConfigCmd())
	return root
}
