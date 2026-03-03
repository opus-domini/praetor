package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/opus-domini/praetor/internal/mcp"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	var projectDir string

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server over stdio",
		Long: `Start a Model Context Protocol server over stdin/stdout.

The MCP server exposes praetor capabilities (plan management, state inspection,
diagnostics, configuration) as tools and resources that any MCP-aware AI agent
can consume. Supported clients include Claude Code, Cursor, VS Code, and others.

The server communicates using JSON-RPC 2.0, one message per line on stdio.
Use 'praetor init' to automatically generate the MCP config for your project.`,
		Example: `  # Add to .mcp.json (Claude Code, Cursor):
  {
    "mcpServers": {
      "praetor": {
        "command": "praetor",
        "args": ["mcp", "--project-dir", "/path/to/project"]
      }
    }
  }

  # VS Code (.vscode/mcp.json):
  {
    "servers": {
      "praetor": {
        "command": "praetor",
        "args": ["mcp", "--project-dir", "${workspaceFolder}"]
      }
    }
  }

  # Or bootstrap automatically:
  praetor init`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if projectDir == "" {
				resolved, err := workspace.ResolveProjectRoot("")
				if err != nil {
					// Fall back to cwd if not in a git repo.
					projectDir, _ = os.Getwd()
				} else {
					projectDir = resolved
				}
			}

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			server := mcp.NewServer(projectDir)
			if err := server.Run(ctx, os.Stdin, os.Stdout); err != nil {
				if ctx.Err() != nil {
					return nil // clean shutdown
				}
				return fmt.Errorf("mcp server: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&projectDir, "project-dir", "", "Project directory (defaults to git root)")
	return cmd
}
