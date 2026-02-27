package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
	agentruntime "github.com/opus-domini/praetor/internal/agent/runtime"
	"github.com/spf13/cobra"
)

func newExecCmd() *cobra.Command {
	var provider string
	var model string
	var codexBin string
	var claudeBin string
	var copilotBin string
	var geminiBin string
	var kimiBin string
	var opencodeBin string
	var openrouterURL string
	var openrouterModel string
	var openrouterKeyEnv string
	var ollamaURL string
	var timeout time.Duration
	var quiet bool

	cmd := &cobra.Command{
		Use:   "exec [prompt]",
		Short: "Run a single prompt on an agent",
		Long: `Run a single prompt on a configured agent and print the response.

Pass the prompt as an argument or pipe it via stdin.`,
		Example: `  praetor exec "Explain this error"
  praetor exec --provider claude "Refactor this function"
  praetor exec --provider copilot "Generate tests for this package"
  praetor exec --provider openrouter --model openai/gpt-4o-mini "Summarize this module"
  praetor exec --provider ollama --model llama3.1 "Summarize this module"
  praetor exec -q "Explain this error"
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

			registry := agentruntime.NewDefaultRegistry(agentruntime.DefaultOptions{
				CodexBin:         codexBin,
				ClaudeBin:        claudeBin,
				CopilotBin:       copilotBin,
				GeminiBin:        geminiBin,
				KimiBin:          kimiBin,
				OpenCodeBin:      opencodeBin,
				OpenRouterURL:    openrouterURL,
				OpenRouterModel:  openrouterModel,
				OpenRouterKeyEnv: openrouterKeyEnv,
				OllamaURL:        ollamaURL,
				OllamaModel:      model,
			})
			agentID := agent.Normalize(provider)
			providerAgent, ok := registry.Get(agentID)
			if !ok {
				return fmt.Errorf("unknown provider %q (supported: claude, codex, copilot, gemini, kimi, opencode, openrouter, ollama)", provider)
			}

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			response, err := providerAgent.Execute(ctx, agent.ExecuteRequest{
				Prompt:  resolvedPrompt,
				Model:   strings.TrimSpace(model),
				Workdir: ".",
				OneShot: true,
			})
			if err != nil {
				return err
			}

			stdout := cmd.OutOrStdout()
			_, err = fmt.Fprintln(stdout, response.Output)
			if err != nil {
				return err
			}

			if !quiet {
				r := NewRenderer(stdout, false)
				r.Dim(formatExecMeta(provider, response.Model, response.DurationS, response.CostUSD))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", string(agent.Codex), "Provider: claude, codex, copilot, gemini, kimi, opencode, openrouter, or ollama")
	cmd.Flags().StringVar(&model, "model", "", "Model name (provider-specific)")
	cmd.Flags().StringVar(&codexBin, "codex-bin", "codex", "Codex binary path or name")
	cmd.Flags().StringVar(&claudeBin, "claude-bin", "claude", "Claude binary path or name")
	cmd.Flags().StringVar(&copilotBin, "copilot-bin", "copilot", "Copilot binary path or name")
	cmd.Flags().StringVar(&geminiBin, "gemini-bin", "gemini", "Gemini CLI binary path or name")
	cmd.Flags().StringVar(&kimiBin, "kimi-bin", "kimi", "Kimi binary path or name")
	cmd.Flags().StringVar(&opencodeBin, "opencode-bin", "opencode", "OpenCode binary path or name")
	cmd.Flags().StringVar(&openrouterURL, "openrouter-url", "https://openrouter.ai/api/v1", "OpenRouter base URL")
	cmd.Flags().StringVar(&openrouterModel, "openrouter-model", "openai/gpt-4o-mini", "Default OpenRouter model when --provider openrouter and --model is empty")
	cmd.Flags().StringVar(&openrouterKeyEnv, "openrouter-api-key-env", "OPENROUTER_API_KEY", "Environment variable containing OpenRouter API key")
	cmd.Flags().StringVar(&ollamaURL, "ollama-url", "http://127.0.0.1:11434", "Ollama base URL when --provider ollama")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Timeout (e.g. 30s, 5m)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Print only the agent output (no metadata)")
	return cmd
}

func formatExecMeta(provider, model string, durationS, costUSD float64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "provider=%s", provider)
	if model != "" {
		fmt.Fprintf(&b, " model=%s", model)
	}
	fmt.Fprintf(&b, " duration=%.1fs", durationS)
	if costUSD > 0 {
		fmt.Fprintf(&b, " cost=$%.4f", costUSD)
	}
	return b.String()
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
