# Claude provider

Go port of `@anthropic-ai/claude-agent-sdk`. Communicates with the `claude` CLI process over the `stream-json` protocol.

## Transport

The provider launches `claude` as a subprocess with:

```
claude --output-format stream-json --verbose --input-format stream-json [flags...]
```

Messages are exchanged as newline-delimited JSON on stdin/stdout. The transport handles process lifecycle, stdin writing, stdout reading, and graceful shutdown.

## SDK surface

### One-shot prompt

```go
message, err := claude.Prompt(ctx, "your prompt", claude.Options{
    Model: "sonnet",
    CWD:   "/path/to/project",
    AllowDangerouslySkipPermissions: true,
})
```

`Prompt()` waits for initialization, sends the user text, closes stdin, and collects the final result message.

### Query lifecycle

For streaming and interactive use:

```go
q, err := claude.NewQuery(ctx, opts)
defer q.Close()

q.WaitInitialized(ctx)         // wait for session ready
q.SendUserText("hello")        // send prompt
q.EndInput()                   // close input stream
result, err := q.AwaitResult(ctx)  // block until completion
```

### Session control

Session control operations are sent as JSON commands on stdin:

- `Interrupt` — cancel current generation
- `SetPermissionMode` — change permission mode
- `SetModel` — change model
- `SetMaxThinkingTokens` — adjust thinking budget
- `ApplyFlagSettings` — apply CLI flag settings
- `StopTask` — stop current task
- `RewindFiles` — revert file changes
- `ReconnectMCPServer` / `ToggleMCPServer` — MCP server management
- `MCPAuthenticate` / `MCPClearAuth` / `MCPServerStatus` / `SetMCPServers` — MCP auth and status

### Callbacks

- `CanUseTool` — permission callback for tool use decisions, returns `PermissionUpdate`
- Hook callbacks — `PreToolUse`, `PostToolUse`, etc.
- `OnMCPMessage` — MCP message handler
- `OnControlRequest` — fallback for unknown control subtypes

### Initialization

```go
q.WaitInitialized(ctx)
init := q.InitializationResult()   // SystemInitMessage
commands := q.SupportedCommands()  // available slash commands
models := q.SupportedModels()      // available models
account := q.AccountInfo()         // account information
```

### Sessions

```go
sessions, err := claude.ListSessions(claude.ListSessionsOptions{
    Dir: "/path/to/project",
})
```

Scans `~/.claude/projects` for persisted session data.

## Type system

### Message types

| Type | Description |
|------|-------------|
| `SDKMessage` | Raw message with `Type` and `Raw` JSON payload |
| `ResultMessage` | Final result with output, cost, usage, duration, errors |
| `AssistantMessage` | Model response with UUID, session ID, parent tool use |
| `SystemInitMessage` | Initialization message with model, CWD, tools, version |
| `StatusMessage` | Status updates (e.g. compacting) |

Parse helpers: `ParseResultMessage()`, `ParseAssistantMessage()`, `ParseSystemInitMessage()`, `ParseStatusMessage()`.

### ResultMessage fields

The result message carries comprehensive execution metadata:

| Field | Type | Description |
|-------|------|-------------|
| `Result` | `string` | Final output text |
| `IsError` | `bool` | Whether the result is an error |
| `TotalCostUSD` | `float64` | Total cost in USD |
| `DurationMS` | `int` | Wall clock duration |
| `DurationAPIMS` | `int` | API-only duration |
| `NumTurns` | `int` | Number of agentic turns |
| `StopReason` | `*string` | Why generation stopped |
| `Usage` | `*NonNullableUsage` | Aggregate token usage |
| `ModelUsage` | `map[string]ModelUsage` | Per-model token usage and cost |
| `PermissionDenials` | `[]PermissionDenial` | Tools that were denied |
| `StructuredOutput` | `json.RawMessage` | JSON schema output |

### Options

Key options for configuring the Claude process:

| Option | Description |
|--------|-------------|
| `Model` | Model to use |
| `CWD` | Working directory |
| `MaxTurns` | Maximum agentic turns |
| `MaxBudgetUSD` | Spending cap |
| `Thinking` | Thinking mode (adaptive, enabled, disabled) |
| `Effort` | Effort level |
| `PermissionMode` | Permission mode (bypass, plan, default) |
| `AllowedTools` / `DisallowedTools` | Tool allowlists/denylists |
| `MCPServers` | MCP server configurations |
| `Sandbox` | Sandbox settings (network, filesystem, ripgrep) |
| `OutputFormat` | Structured output format (JSON schema) |
| `CanUseTool` | Permission callback |
| `Plugins` | Local plugin directories |

### Permission system

`PermissionUpdate` supports six operation types:

| Type | Description |
|------|-------------|
| `addRules` | Add permission rules with behavior (allow/deny) |
| `replaceRules` | Replace all rules |
| `removeRules` | Remove specific rules |
| `setMode` | Change permission mode |
| `addDirectories` | Add allowed directories |
| `removeDirectories` | Remove allowed directories |

### Sandbox settings

Typed configuration for the Claude Code sandbox:

```go
claude.SandboxSettings{
    Enabled: boolPtr(true),
    Network: &claude.SandboxNetworkConfig{
        AllowedDomains: []string{"api.example.com"},
    },
    Filesystem: &claude.SandboxFilesystemConfig{
        AllowWrite: []string{"/tmp"},
    },
}
```

## Known limitations

- No in-process MCP SDK server support (`createSdkMcpServer` from TypeScript SDK).
- No unstable v2 session API (`unstable_v2_createSession`).
- Type surface focuses on practical query/session control usage.

## Orchestrator integration

Adapted to the orchestrator contract through:

```go
provider := claude.NewProvider(claude.Options{})
provider.ID()                          // "claude"
provider.Run(ctx, orchestrator.Request{Prompt: "..."})
```
