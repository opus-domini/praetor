# Claude Provider

Go port of `@anthropic-ai/claude-agent-sdk`, communicating with the `claude` process over `stream-json`.

## Current coverage

Implemented:

- process transport (`claude`) with `--input-format stream-json` and `--output-format stream-json`
- user prompt streaming and SDK message handling
- one-shot prompt helper: `Prompt(...)`
- session control operations:
  - `Interrupt`
  - `SetPermissionMode`
  - `SetModel`
  - `SetMaxThinkingTokens`
  - `ApplyFlagSettings`
  - `StopTask`
  - `RewindFiles`
  - `ReconnectMCPServer`
  - `ToggleMCPServer`
  - `MCPAuthenticate`
  - `MCPClearAuth`
  - `MCPServerStatus`
  - `SetMCPServers`
- callback support:
  - `CanUseTool`
  - hook callbacks (`hook_callback`)
  - `mcp_message`
  - fallback `OnControlRequest` for unknown subtypes
- asynchronous initialization helpers:
  - `WaitInitialized`
  - `InitializationResult`
  - `SupportedCommands`
  - `SupportedModels`
  - `AccountInfo`
- persisted local sessions:
  - `ListSessions(...)`
  - scanning `~/.claude/projects`
  - directory filter including detected Git worktrees

## Known limitations

- no in-process MCP SDK server support (`createSdkMcpServer` from the TypeScript SDK)
- no unstable v2 API support (`unstable_v2_createSession`, etc.)
- type surface intentionally focuses on practical query/session control usage

## Adapter integration

This provider is adapted to the orchestrator contract through:

- `NewProvider(Options)`
- `Provider.Run(ctx, orchestrator.Request)`
