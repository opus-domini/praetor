package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/orchestrator"
	"github.com/opus-domini/praetor/internal/providers/claude"
	"github.com/opus-domini/praetor/internal/providers/codex"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	var provider string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "exec [prompt]",
		Short: "Run a single prompt on a provider",
		Long: `Run a single prompt on a provider and print the response.

Pass the prompt as an argument or pipe it via stdin.`,
		Example: `  praetor exec "Explain this error"
  praetor exec --provider claude "Refactor this function"
  echo "Reply with OK" | praetor exec`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if timeout < 0 {
				return errors.New("timeout cannot be negative")
			}

			prompt := ""
			if len(args) > 0 {
				prompt = args[0]
			}
			stdin := cmd.InOrStdin()
			resolvedPrompt, err := readPrompt(prompt, stdin, isInteractiveInput(stdin))
			if err != nil {
				return err
			}

			providerID := orchestrator.ProviderID(strings.ToLower(strings.TrimSpace(provider)))
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

	cmd.Flags().StringVar(&provider, "provider", string(orchestrator.ProviderCodex), "Provider: codex or claude")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Timeout (e.g. 30s, 5m)")
	return cmd
}

func buildProvider(id orchestrator.ProviderID) (orchestrator.Provider, error) {
	switch id {
	case orchestrator.ProviderClaude:
		return claude.NewProvider(claude.Options{}), nil
	case orchestrator.ProviderCodex:
		return codex.NewProvider(codex.CodexOptions{})
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: claude, codex)", id)
	}
}

func readPrompt(flagPrompt string, in io.Reader, interactive bool) (string, error) {
	if prompt := strings.TrimSpace(flagPrompt); prompt != "" {
		return prompt, nil
	}
	if interactive {
		return "", errors.New("prompt is required when stdin is interactive: pass as argument or pipe via stdin")
	}

	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", errors.New("prompt is required: pass as argument or pipe via stdin")
	}
	return prompt, nil
}

func isInteractiveInput(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
