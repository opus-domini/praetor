package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/orchestrator"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var provider string
	var prompt string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one prompt on a provider",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolvedPrompt, err := readPrompt(prompt, cmd.InOrStdin())
			if err != nil {
				return err
			}

			providerID := orchestrator.ProviderID(strings.TrimSpace(provider))
			p, err := buildProvider(providerID)
			if err != nil {
				return err
			}

			registry := orchestrator.NewRegistry()
			if err := registry.Register(p); err != nil {
				return err
			}

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			engine := orchestrator.New(registry)
			result, err := engine.Run(ctx, orchestrator.Request{
				Provider: providerID,
				Prompt:   resolvedPrompt,
			})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Response)
			return err
		},
	}

	cmd.Flags().StringVar(&provider, "provider", string(orchestrator.ProviderCodex), "Provider to execute: codex or claude")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Prompt text. If empty, the command reads stdin")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Optional timeout, for example: 30s")
	return cmd
}

func readPrompt(flagPrompt string, in io.Reader) (string, error) {
	if prompt := strings.TrimSpace(flagPrompt); prompt != "" {
		return prompt, nil
	}

	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", errors.New("prompt is required: pass --prompt or pipe text via stdin")
	}
	return prompt, nil
}
