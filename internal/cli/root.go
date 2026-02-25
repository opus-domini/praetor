package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/opus-domini/praetor/internal/orchestrator"
	"github.com/opus-domini/praetor/internal/providers/claude"
	"github.com/opus-domini/praetor/internal/providers/codex"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the praetor CLI root command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "praetor",
		Short:         "Orchestrate AI providers from one CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newRunCmd())
	root.AddCommand(newProvidersCmd())
	return root
}

func knownProviders() []orchestrator.ProviderID {
	ids := []orchestrator.ProviderID{
		orchestrator.ProviderClaude,
		orchestrator.ProviderCodex,
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func buildProvider(id orchestrator.ProviderID) (orchestrator.Provider, error) {
	switch id {
	case orchestrator.ProviderClaude:
		return claude.NewProvider(claude.Options{}), nil
	case orchestrator.ProviderCodex:
		return codex.NewProvider(codex.CodexOptions{})
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: %s)", id, joinProviderIDs(knownProviders()))
	}
}

func joinProviderIDs(ids []orchestrator.ProviderID) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, string(id))
	}
	return strings.Join(parts, ", ")
}
