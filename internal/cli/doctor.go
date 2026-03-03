package cli

import (
	"context"
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

			r.Header("Agent Health Check")
			_, _ = fmt.Fprintln(stdout)

			entries := agent.AllCatalogEntries()
			healthy := 0
			total := len(entries)

			for _, entry := range entries {
				writeProbeProgress(stdout, r, entry)
				binOverride := binaryOverrides[entry.ID]
				restEndpoint := restEndpoints[entry.ID]
				result, _ := prober.ProbeOne(ctx, entry.ID, binOverride, restEndpoint)
				clearProbeProgress(stdout, r, entry)
				writeProbeResult(stdout, r, result)
				if result.Healthy() {
					healthy++
				}
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
	case agent.StatusOK:
		statusColor = "32" // green
	case agent.StatusNotFound:
		statusColor = "31" // red
	case agent.StatusUnreachable:
		statusColor = "33" // yellow
	default:
		statusColor = "31" // red
	}

	transportTag := strings.ToUpper(string(result.Transport))

	// Build the dots padding for alignment.
	name := result.DisplayName
	padLen := 28 - len(name)
	if padLen < 3 {
		padLen = 3
	}
	dots := strings.Repeat(".", padLen)

	// Format: "  Claude Code .............. [CLI]  v1.0.30"
	detail := result.Detail
	if detail == "" {
		detail = string(result.Status)
	}

	_, _ = fmt.Fprintf(w, "  %s %s %s[%s]%s  %s%s%s\n",
		name,
		r.c("2")+dots+r.reset(),
		r.c("36"), transportTag, r.reset(),
		r.c(statusColor), detail, r.reset(),
	)

	// Always show a second line for consistent two-line format.
	// If agent is found: show path/endpoint.
	// If not found: show install hint.
	secondLine := result.Path
	if secondLine == "" {
		entry, ok := agent.LookupCatalog(result.ID)
		if ok && entry.InstallHint != "" {
			secondLine = entry.InstallHint
		}
	}
	if secondLine == "" {
		secondLine = "-"
	}
	_, _ = fmt.Fprintf(w, "  %s%s  %s%s\n",
		strings.Repeat(" ", len(name)),
		r.c("2")+strings.Repeat(" ", padLen)+r.reset(),
		r.c("2"), secondLine+r.reset(),
	)
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
