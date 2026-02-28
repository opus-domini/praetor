package cli

import (
	"os"

	tmuxruntime "github.com/opus-domini/praetor/internal/runtime/tmux"
	"github.com/spf13/cobra"
)

func newFmtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "fmt",
		Short:  "Format agent JSONL stream for live display",
		Long:   `Reads JSONL from stdin (codex or claude stream-json) and writes human-readable, ANSI-formatted text to stdout. Used internally by the tmux runner to make pane output readable.`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			f := tmuxruntime.NewStreamFormatter(cmd.OutOrStdout())
			// Best-effort display: never fail the pipeline over formatting errors.
			_ = f.Format(os.Stdin)
			return nil
		},
	}
	return cmd
}
