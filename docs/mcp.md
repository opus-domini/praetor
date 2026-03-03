# MCP Server

Praetor includes a built-in [Model Context Protocol](https://modelcontextprotocol.io/) server, enabling any MCP-aware AI agent (Claude Code, Cursor, etc.) to interact with plans, state, and diagnostics programmatically.

## Starting the server

```bash
praetor mcp [--project-dir <path>]
```

The server communicates over **stdio** using JSON-RPC 2.0 (one message per line). It is designed to be referenced in `.mcp.json`:

```json
{
  "mcpServers": {
    "praetor": {
      "command": "praetor",
      "args": ["mcp", "--project-dir", "/path/to/project"]
    }
  }
}
```

## Tools

### Plan management

| Tool | Description | Required params |
|---|---|---|
| `plan_list` | List all plans for the current project | - |
| `plan_show` | Show a plan's full JSON content | `slug` |
| `plan_status` | Get detailed status for a plan | `slug` |
| `plan_create` | Create a new plan from a name | `name` |
| `plan_reset` | Reset a plan's runtime state | `slug` |

### State and diagnostics

| Tool | Description | Required params |
|---|---|---|
| `plan_events` | Get execution events from a plan run | `slug` |
| `plan_diagnose` | Get diagnostics (errors, stalls, costs) | `slug` |

The `plan_diagnose` tool accepts a `query` parameter: `errors`, `stalls`, `fallbacks`, `costs`, or `all` (default).

### Configuration

| Tool | Description | Required params |
|---|---|---|
| `config_show` | Show resolved configuration | - |
| `config_set` | Set a configuration value | `key`, `value` |

### Execution

| Tool | Description | Required params |
|---|---|---|
| `doctor` | Check availability of all AI agent providers | - |

## Resources

The server also exposes MCP resources for passive data access:

| URI | Description |
|---|---|
| `praetor://plans` | List of all plans |
| `praetor://plans/{slug}` | Full plan JSON |
| `praetor://plans/{slug}/state` | Current execution state |
| `praetor://config` | Resolved configuration |
| `praetor://agents` | Agent health status |

## Example interaction

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"claude-code","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"plan_list","arguments":{}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"doctor","arguments":{}}}
{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"praetor://config"}}
```

## Implementation

The MCP server is implemented in `internal/mcp/` using only Go stdlib:

- `server.go` — JSON-RPC 2.0 stdio loop and MCP dispatch
- `protocol.go` — MCP protocol types
- `tools.go` — Tool registry and helpers
- `tools_plan.go` — Plan management tools
- `tools_state.go` — State and diagnostics tools
- `tools_config.go` — Configuration tools
- `tools_exec.go` — Execution tools (doctor)
- `resources.go` — MCP resource definitions

All tool handlers reuse existing praetor packages (`state.Store`, `domain.LoadPlan`, `config.LoadResolved`, etc.) ensuring consistent behavior with the CLI.
