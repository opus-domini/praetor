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

			r := NewRenderer(cmd.OutOrStdout(), noColor)

			projectRoot, err := workspace.ResolveProjectRoot("")
			if err != nil {
				projectRoot, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve project root: %w", err)
				}
			}

			// --- Interactive agent selection ---
			var agents []string
			if shouldUseInitSelector(cmd.InOrStdin(), cmd.OutOrStdout()) {
				selected, err := runInitSelector(cmd.InOrStdin(), cmd.OutOrStdout(), projectRoot)
				if err != nil {
					return err
				}
				agents = selected
			} else {
				agents = detectAgents(projectRoot)
				if len(agents) == 0 {
					agents = commands.SupportedAgents
				}
			}

			// Banner.
			r.Banner("Praetor", fmt.Sprintf("Installing into %s", projectRoot))

			// --- Scan phase ---
			mcpTargets := detectMCPTargets(projectRoot)

			r.Info(fmt.Sprintf("Selected agents: %s", strings.Join(agents, ", ")))

			mcpLabels := make([]string, len(mcpTargets))
			for i, t := range mcpTargets {
				rel, _ := filepath.Rel(projectRoot, t)
				if rel == "" {
					rel = t
				}
				mcpLabels[i] = rel
			}
			r.Info(fmt.Sprintf("MCP targets: %s", strings.Join(mcpLabels, ", ")))

			// --- Step 1: Agent commands ---
			r.Step(1, 2, "Agent Commands")

			if err := commands.Sync(projectRoot, agents); err != nil {
				return fmt.Errorf("sync commands: %w", err)
			}

			cmdCount := len(commands.DefaultCommands())
			r.Success(fmt.Sprintf("Generated %d commands in .agents/commands/", cmdCount))

			for _, a := range agents {
				r.Hint(fmt.Sprintf(".%s/commands/ -> .agents/commands/", a))
			}

			// --- Step 2: MCP server ---
			r.Step(2, 2, "MCP Server")

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
					r.Warn(fmt.Sprintf("%s — %v", rel, err))
					continue
				}
				if wrote {
					r.Success(fmt.Sprintf("Registered in %s", rel))
				} else {
					r.Info(fmt.Sprintf("Already registered in %s", rel))
				}
			}

			// --- Done ---
			r.Done("Praetor is ready!")

			r.Dim("  Next steps:")
			r.Hint("praetor doctor              Check agent availability")
			r.Hint("praetor plan create \"...\"   Create your first plan")
			r.Hint("praetor plan run <slug>     Execute the plan")
			r.Blank()

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
