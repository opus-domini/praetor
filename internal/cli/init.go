package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/commands"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var noColor bool
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Install praetor into the current project",
		Long: `Set up praetor in an existing project by detecting installed AI agents
and registering shared commands and MCP server configuration.

This command scans the project for agent directories (.claude/, .cursor/,
.codex/) and editor configs (.vscode/), then:

  1. Generates shared agent commands in .agents/commands/
  2. Creates symlinks from each detected agent directory
  3. Registers the MCP server in .mcp.json (and .vscode/mcp.json if applicable)

The command is idempotent — running it again updates everything safely.`,
		Example: `  praetor init
  praetor init --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			w := cmd.OutOrStdout()
			r := NewRenderer(w, noColor)

			projectRoot, err := workspace.ResolveProjectRoot("")
			if err != nil {
				projectRoot, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve project root: %w", err)
				}
			}

			// Banner.
			_, _ = fmt.Fprintf(w, "\n  %sPraetor%s — Installing into %s%s%s\n",
				r.c("1;36"), r.reset(),
				r.c("1"), projectRoot, r.reset())

			// --- Scan phase -----------------------------------------------------------
			_, _ = fmt.Fprintf(w, "\n  %sScanning project...%s\n", r.c("2"), r.reset())

			agents := detectAgents(projectRoot)
			mcpTargets := detectMCPTargets(projectRoot)

			if len(agents) > 0 {
				_, _ = fmt.Fprintf(w, "  %s✔%s Detected agents: %s%s%s\n",
					r.c("32"), r.reset(),
					r.c("1"), strings.Join(agents, ", "), r.reset())
			} else {
				_, _ = fmt.Fprintf(w, "  %s•%s No agent directories found, using defaults: %s%s%s\n",
					r.c("2"), r.reset(),
					r.c("1"), strings.Join(commands.SupportedAgents, ", "), r.reset())
				agents = commands.SupportedAgents
			}

			mcpLabels := make([]string, len(mcpTargets))
			for i, t := range mcpTargets {
				rel, _ := filepath.Rel(projectRoot, t)
				if rel == "" {
					rel = t
				}
				mcpLabels[i] = rel
			}
			_, _ = fmt.Fprintf(w, "  %s✔%s MCP targets: %s%s%s\n",
				r.c("32"), r.reset(),
				r.c("1"), strings.Join(mcpLabels, ", "), r.reset())

			// --- Step 1: Agent commands -----------------------------------------------
			stepNum := 1
			totalSteps := 2
			_, _ = fmt.Fprintf(w, "\n  %s[%d/%d]%s %sAgent Commands%s\n",
				r.c("1;34"), stepNum, totalSteps, r.reset(),
				r.c("1"), r.reset())

			if err := commands.Sync(projectRoot, agents); err != nil {
				return fmt.Errorf("sync commands: %w", err)
			}

			cmdNames := commands.DefaultCommands()
			_, _ = fmt.Fprintf(w, "  %s✔%s Generated %d commands in .agents/commands/\n",
				r.c("32"), r.reset(), len(cmdNames))

			for _, a := range agents {
				_, _ = fmt.Fprintf(w, "    %s.%s/commands/%s → .agents/commands/\n",
					r.c("2"), a, r.reset())
			}

			// --- Step 2: MCP server ---------------------------------------------------
			stepNum++
			_, _ = fmt.Fprintf(w, "\n  %s[%d/%d]%s %sMCP Server%s\n",
				r.c("1;34"), stepNum, totalSteps, r.reset(),
				r.c("1"), r.reset())

			entry := mcpServerEntry{
				Command: "praetor",
				Args:    []string{"mcp", "--project-dir", projectRoot},
			}

			for _, target := range mcpTargets {
				wrote, err := writeMCPConfig(target, entry, force)
				rel, _ := filepath.Rel(projectRoot, target)
				if rel == "" {
					rel = target
				}
				if err != nil {
					_, _ = fmt.Fprintf(w, "  %s✗%s %s — %v\n",
						r.c("33"), r.reset(), rel, err)
					continue
				}
				if wrote {
					_, _ = fmt.Fprintf(w, "  %s✔%s Registered in %s\n",
						r.c("32"), r.reset(), rel)
				} else {
					_, _ = fmt.Fprintf(w, "  %s•%s Already registered in %s\n",
						r.c("2"), r.reset(), rel)
				}
			}

			// --- Done -----------------------------------------------------------------
			_, _ = fmt.Fprintf(w, "\n  %s✔ Praetor is ready!%s\n", r.c("1;32"), r.reset())

			_, _ = fmt.Fprintf(w, "\n  %sNext steps:%s\n", r.c("2"), r.reset())
			_, _ = fmt.Fprintf(w, "    praetor doctor              %sCheck agent availability%s\n",
				r.c("2"), r.reset())
			_, _ = fmt.Fprintf(w, "    praetor plan create %s\"...\"%s   %sCreate your first plan%s\n",
				r.c("2"), r.reset(), r.c("2"), r.reset())
			_, _ = fmt.Fprintf(w, "    praetor plan run %s<slug>%s     %sExecute the plan%s\n",
				r.c("2"), r.reset(), r.c("2"), r.reset())
			_, _ = fmt.Fprintln(w)

			return nil
		},
	}

	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing MCP config entries")
	return cmd
}

// detectAgents finds agent directories (.claude/, .cursor/, .codex/) present in the project.
func detectAgents(projectRoot string) []string {
	var found []string
	for _, name := range commands.SupportedAgents {
		dir := filepath.Join(projectRoot, "."+name)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			found = append(found, name)
		}
	}
	return found
}

// mcpConfig represents the .mcp.json structure.
type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// detectMCPTargets finds MCP config locations based on present editor directories.
func detectMCPTargets(projectRoot string) []string {
	var targets []string

	// .mcp.json at project root (Claude Code, Cursor).
	targets = append(targets, filepath.Join(projectRoot, ".mcp.json"))

	// VS Code workspace MCP config.
	vscodeDir := filepath.Join(projectRoot, ".vscode")
	if info, err := os.Stat(vscodeDir); err == nil && info.IsDir() {
		targets = append(targets, filepath.Join(vscodeDir, "mcp.json"))
	}

	return targets
}

// writeMCPConfig writes or merges an MCP server entry into the target file.
// Returns true if the file was written, false if praetor was already registered.
func writeMCPConfig(target string, entry mcpServerEntry, force bool) (bool, error) {
	var cfg mcpConfig

	data, err := os.ReadFile(target)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			if !force {
				return false, fmt.Errorf("parse existing file: %w (use --force to overwrite)", err)
			}
			cfg = mcpConfig{}
		}
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]mcpServerEntry)
	}

	if _, exists := cfg.MCPServers["praetor"]; exists && !force {
		return false, nil
	}

	cfg.MCPServers["praetor"] = entry

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')

	return true, os.WriteFile(target, out, 0o644)
}
