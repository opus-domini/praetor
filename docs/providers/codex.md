# Codex provider

Go port of `@openai/codex-sdk`. Communicates with the `codex` CLI process over a JSONL event stream.

## Transport

The provider launches `codex exec` as a subprocess with `--experimental-json` for structured output. Events are parsed from stdout as newline-delimited JSON.

## SDK surface

### Client and threads

```go
client, err := codex.New(codex.CodexOptions{})

thread := client.StartThread(&codex.ThreadOptions{
    WorkingDirectory: "/path/to/project",
    SandboxMode:      codex.SandboxModeWorkspaceWrite,
    ApprovalPolicy:   codex.ApprovalModeNever,
    Model:            "o4-mini",
})

turn, err := thread.Run(ctx, "implement feature X", nil)
fmt.Println(turn.FinalResponse)
```

### Thread options

| Option | Description |
|--------|-------------|
| `WorkingDirectory` | CWD for the codex process |
| `Model` | Model to use |
| `SandboxMode` | Sandbox policy: `off`, `workspace-write`, `workspace-read` |
| `ApprovalPolicy` | Tool approval: `never`, `unless-allow-listed`, `on-failure` |
| `SkipGitRepoCheck` | Skip git repository validation |
| `WebSearchEnabled` | Enable web search capability |
| `AdditionalDirectories` | Extra directories to include |
| `Config` | Key-value configuration overrides |
| `OutputSchema` | JSON schema for structured output |

### Resuming threads

```go
threadID, ok := thread.ID()
if ok {
    resumed := client.ResumeThread(threadID, &codex.ThreadOptions{...})
    turn, err := resumed.Run(ctx, "follow-up prompt", nil)
}
```

`Thread.ID()` returns `(string, bool)` — the second value is `false` before the first turn completes.

### Streaming

```go
turn, err := thread.RunStreamed(ctx, "prompt", nil, func(item codex.StreamItem) {
    switch item.Type {
    case "agent_message":
        fmt.Println(item.AgentMessage.Content)
    case "file_change":
        fmt.Println(item.FileChange.Path)
    }
})
```

### Stream item types

| Type | Description |
|------|-------------|
| `agent_message` | Model text response |
| `reasoning` | Chain-of-thought reasoning |
| `command_execution` | Shell command execution |
| `file_change` | File creation, modification, or deletion |
| `mcp_tool_call` | MCP tool invocation |
| `web_search` | Web search result |
| `todo_list` | Task list update |
| `error` | Error message |

### Input types

```go
// Text input
turn, err := thread.Run(ctx, "hello", nil)

// Structured input with images
turn, err := thread.Run(ctx, "", []codex.UserInput{
    {Type: "text", Text: "describe this image"},
    {Type: "image", ImagePath: "/path/to/image.png"},
})
```

## Type system

### Status constants

```go
// Patch apply status
codex.PatchApplyStatusCompleted  // "completed"
codex.PatchApplyStatusFailed     // "failed"

// MCP tool call status
codex.McpToolCallStatusInProgress  // "in_progress"
codex.McpToolCallStatusCompleted   // "completed"
codex.McpToolCallStatusFailed      // "failed"
```

### FileUpdateChange

File changes include a `Status` field indicating the patch apply result:

```go
type FileUpdateChange struct {
    Type    string `json:"type"`
    Path    string `json:"path"`
    Content string `json:"content,omitempty"`
    Patch   string `json:"patch,omitempty"`
    Status  string `json:"status,omitempty"` // PatchApplyStatus
}
```

## Pipeline integration

In the pipeline runner, Codex runs with `--json` output mode (tmux runtime). The raw JSON output is post-processed to extract:

- `.result` — the agent's text response
- `.total_cost_usd` — invocation cost for the tracking ledger

Raw JSON is preserved as `<prefix>.raw.json` in the run log directory.

## Agent integration

Adapted to the agent registry through:

```go
provider, err := codex.NewProvider(codex.CodexOptions{})
provider.ID()                          // "codex"
provider.Run(ctx, providers.Request{Prompt: "..."})
```
