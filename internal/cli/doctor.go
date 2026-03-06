package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var workdir string
	var noColor bool
	var timeout time.Duration
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check agent availability and health",
		Long: `Check the health and availability of all configured agent backends.

For CLI agents: verifies the binary exists in PATH and reads its version.
For REST agents: verifies the endpoint is reachable via HTTP health check.`,
		Example: `  praetor doctor
  praetor doctor --no-color
  praetor doctor --timeout 15s`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			stdout := cmd.OutOrStdout()
			r := NewRenderer(stdout, noColor)

			// Load config for binary/endpoint overrides.
			projectRoot := "."
			if resolved, err := workspace.ResolveProjectRoot(strings.TrimSpace(workdir)); err == nil {
				projectRoot = resolved
			}
			cfg, _ := config.Load(projectRoot) // ignore error, proceed with defaults

			binaryOverrides := buildBinaryOverrides(cfg)
			restEndpoints := buildRESTEndpoints(cfg)

			probeTimeout := timeout
			if probeTimeout <= 0 {
				probeTimeout = 10 * time.Second
			}
			prober := agent.NewProber(agent.WithTimeout(probeTimeout))

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout+5*time.Second) // extra grace for overall command
				defer cancel()
			}

			if !jsonOut {
				r.Header("Agent Health Check")
				_, _ = fmt.Fprintln(stdout)
			}

			entries := agent.AllCatalogEntries()
			healthy := 0
			total := len(entries)
			results := make([]agent.ProbeResult, 0, len(entries))

			for _, entry := range entries {
				if !jsonOut {
					writeProbeProgress(stdout, r, entry)
				}
				binOverride := binaryOverrides[entry.ID]
				restEndpoint := restEndpoints[entry.ID]
				result, _ := prober.ProbeOne(ctx, entry.ID, binOverride, restEndpoint)
				results = append(results, result)
				if !jsonOut {
					clearProbeProgress(stdout, r, entry)
				}
				if !jsonOut {
					writeProbeResult(stdout, r, result)
				}
				if result.Healthy() {
					healthy++
				}
			}

			if jsonOut {
				encoded, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return err
				}
				encoded = append(encoded, '\n')
				_, err = stdout.Write(encoded)
				return err
			}

			_, _ = fmt.Fprintln(stdout)
			summary := fmt.Sprintf("%d/%d agents available", healthy, total)
			if healthy == total {
				r.Success(summary)
			} else if healthy > 0 {
				r.Warn(summary)
			} else {
				r.Error(summary)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&workdir, "workdir", ".", "Working directory for config resolution")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "Per-probe timeout (e.g. 5s, 15s)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print structured JSON output")
	return cmd
}

func writeProbeProgress(w io.Writer, r *Renderer, entry agent.CatalogEntry) {
	_, _ = fmt.Fprintf(w, "  %sChecking %s...%s\r",
		r.c("2"), entry.DisplayName, r.reset())
}

func clearProbeProgress(w io.Writer, r *Renderer, entry agent.CatalogEntry) {
	// Overwrite the "Checking..." line with spaces then return carriage.
	width := len("  Checking ...") + len(entry.DisplayName) + 10
	_, _ = fmt.Fprintf(w, "\r%s\r", strings.Repeat(" ", width))
}

func writeProbeResult(w io.Writer, r *Renderer, result agent.ProbeResult) {
	var statusColor string
	switch result.Status {
	case agent.StatusPass:
		statusColor = "32" // green
	case agent.StatusWarn:
		statusColor = "33" // yellow
	default:
		statusColor = "31" // red
	}

	// Build the dots padding for alignment.
	name := result.DisplayName
	padLen := 28 - len(name)
	if padLen < 3 {
		padLen = 3
	}
	dots := strings.Repeat(".", padLen)
	transportTag := strings.ToUpper(string(result.Transport))

	_, _ = fmt.Fprintf(w, "  %s %s %s%s%s [%s]\n",
		name,
		r.c("2")+dots+r.reset(),
		r.c(statusColor), strings.TrimSpace(string(result.Status)), r.reset(),
		transportTag,
	)

	checks := result.Checks
	if len(checks) == 0 {
		level := "info"
		if result.Status == agent.StatusWarn {
			level = "warn"
		}
		if result.Status == agent.StatusFail {
			level = "error"
		}
		if strings.TrimSpace(result.Detail) != "" {
			checks = append(checks, agent.HealthCheck{Level: level, Message: strings.TrimSpace(result.Detail)})
		}
		if strings.TrimSpace(result.Path) != "" {
			checks = append(checks, agent.HealthCheck{Level: "info", Message: "Location", Detail: strings.TrimSpace(result.Path)})
		} else if entry, ok := agent.LookupCatalog(result.ID); ok && strings.TrimSpace(entry.InstallHint) != "" {
			checks = append(checks, agent.HealthCheck{Level: "info", Message: "Install hint", Detail: strings.TrimSpace(entry.InstallHint)})
		}
	}
	for _, check := range checks {
		level := strings.ToLower(strings.TrimSpace(check.Level))
		color := "2"
		switch level {
		case "warn":
			color = "33"
		case "error":
			color = "31"
		}
		detail := strings.TrimSpace(check.Message)
		if strings.TrimSpace(check.Detail) != "" {
			detail = detail + ": " + strings.TrimSpace(check.Detail)
		}
		_, _ = fmt.Fprintf(w, "    %s[%s]%s %s\n", r.c(color), level, r.reset(), detail)
		if strings.TrimSpace(check.Hint) != "" {
			_, _ = fmt.Fprintf(w, "    %s[hint]%s %s\n", r.c("36"), r.reset(), strings.TrimSpace(check.Hint))
		}
	}
}

func buildBinaryOverrides(cfg config.Config) map[agent.ID]string {
	overrides := make(map[agent.ID]string)
	if cfg.CodexBin != "" {
		overrides[agent.Codex] = cfg.CodexBin
	}
	if cfg.ClaudeBin != "" {
		overrides[agent.Claude] = cfg.ClaudeBin
	}
	if cfg.CopilotBin != "" {
		overrides[agent.Copilot] = cfg.CopilotBin
	}
	if cfg.GeminiBin != "" {
		overrides[agent.Gemini] = cfg.GeminiBin
	}
	if cfg.KimiBin != "" {
		overrides[agent.Kimi] = cfg.KimiBin
	}
	if cfg.OpenCodeBin != "" {
		overrides[agent.OpenCode] = cfg.OpenCodeBin
	}
	return overrides
}

func buildRESTEndpoints(cfg config.Config) map[agent.ID]string {
	endpoints := make(map[agent.ID]string)
	if cfg.OpenRouterURL != "" {
		endpoints[agent.OpenRouter] = cfg.OpenRouterURL
	}
	if cfg.OllamaURL != "" {
		endpoints[agent.Ollama] = cfg.OllamaURL
	}
	if cfg.LMStudioURL != "" {
		endpoints[agent.LMStudio] = cfg.LMStudioURL
	}
	return endpoints
}
