package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List available providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, providerID := range knownProviders() {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), providerID); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
