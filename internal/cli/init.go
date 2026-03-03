package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opus-domini/praetor/internal/commands"
	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var agents []string
	var noColor bool
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a project for AI agent integration",
		Long: `Initialize a project with praetor configuration, shared agent commands,
and MCP server registration in one step.

This command is idempotent — running it again updates commands and MCP config
without overwriting the global config file (unless --force is used).

Steps performed:
  1. Create a config file if none exists (praetor config init)
  2. Generate shared agent commands and symlinks (praetor commands sync)
  3. Write .mcp.json for detected MCP clients (Claude Code, Cursor, VS Code)`,
		Example: `  # Bootstrap everything with defaults
  praetor init

  # Bootstrap for specific agents only
  praetor init --agents claude,cursor

  # Overwrite existing config file
  praetor init --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			r := NewRenderer(cmd.OutOrStdout(), noColor)

			projectRoot, err := workspace.ResolveProjectRoot("")
			if err != nil {
				// Fall back to cwd if not in a git repo.
				projectRoot, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve project root: %w", err)
				}
			}

			// Step 1: Config init.
			initConfig(r, force)

			// Step 2: Commands sync.
			if err := initCommands(r, projectRoot, agents); err != nil {
				return err
			}

			// Step 3: MCP config.
			if err := initMCP(r, projectRoot, force); err != nil {
				return err
			}

			// Next steps.
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			r.Header("Next steps")
			r.Info("Edit config:    praetor config edit")
			r.Info("Check agents:   praetor doctor")
			r.Info("Create a plan:  praetor plan create \"your objective\"")
			r.Info("Run the plan:   praetor plan run <slug>")

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&agents, "agents", commands.SupportedAgents, "Agent directories to create symlinks for")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config and MCP files")
	return cmd
}

func initConfig(r *Renderer, force bool) {
	cfgPath := config.Path()
	if cfgPath == "" {
		r.Warn("Cannot determine config file path, skipping config init")
		return
	}

	if !force {
		if _, err := os.Stat(cfgPath); err == nil {
			r.Info(fmt.Sprintf("Config exists: %s (use --force to overwrite)", cfgPath))
			return
		}
	}

	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		r.Warn(fmt.Sprintf("Create config directory: %v", err))
		return
	}

	if err := os.WriteFile(cfgPath, []byte(config.Template()), 0o644); err != nil {
		r.Warn(fmt.Sprintf("Write config file: %v", err))
		return
	}

	r.Success(fmt.Sprintf("Config created: %s", cfgPath))
}

func initCommands(r *Renderer, projectRoot string, agents []string) error {
	if err := commands.Sync(projectRoot, agents); err != nil {
		return fmt.Errorf("commands sync: %w", err)
	}

	r.Success("Commands synced to .agents/commands/")

	active := agents
	if len(active) == 0 {
		active = commands.SupportedAgents
	}
	for _, a := range active {
		r.Info(fmt.Sprintf(".%s/commands/ -> .agents/commands/", a))
	}
	return nil
}

// mcpConfig represents the .mcp.json structure.
type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func initMCP(r *Renderer, projectRoot string, force bool) error {
	entry := mcpServerEntry{
		Command: "praetor",
		Args:    []string{"mcp", "--project-dir", projectRoot},
	}

	targets := detectMCPTargets(projectRoot)
	if len(targets) == 0 {
		// Default: write .mcp.json at project root.
		targets = []string{filepath.Join(projectRoot, ".mcp.json")}
	}

	for _, target := range targets {
		if err := writeMCPConfig(target, entry, force); err != nil {
			r.Warn(fmt.Sprintf("Write %s: %v", target, err))
			continue
		}

		rel, _ := filepath.Rel(projectRoot, target)
		if rel == "" {
			rel = target
		}
		r.Success(fmt.Sprintf("MCP config written to %s", rel))
	}

	return nil
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
func writeMCPConfig(target string, entry mcpServerEntry, force bool) error {
	var cfg mcpConfig

	// Try to load existing config.
	data, err := os.ReadFile(target)
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			if !force {
				return fmt.Errorf("parse existing file: %w (use --force to overwrite)", err)
			}
			cfg = mcpConfig{}
		}
	}

	if cfg.MCPServers == nil {
		cfg.MCPServers = make(map[string]mcpServerEntry)
	}

	// Don't overwrite an existing praetor entry unless --force.
	if _, exists := cfg.MCPServers["praetor"]; exists && !force {
		return nil // already configured
	}

	cfg.MCPServers["praetor"] = entry

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	return os.WriteFile(target, out, 0o644)
}
