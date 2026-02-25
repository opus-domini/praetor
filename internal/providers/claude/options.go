package claude

import (
	"context"
	"encoding/json"
	"io"
)

// PermissionMode controls how tool permission prompts are handled.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
)

// ExitReason mirrors SDK exit reasons.
type ExitReason string

const (
	ExitReasonClear                     ExitReason = "clear"
	ExitReasonLogout                    ExitReason = "logout"
	ExitReasonPromptInputExit           ExitReason = "prompt_input_exit"
	ExitReasonOther                     ExitReason = "other"
	ExitReasonBypassPermissionsDisabled ExitReason = "bypass_permissions_disabled"
)

// HookEvent mirrors the SDK hook event names.
type HookEvent string

const (
	HookEventPreToolUse         HookEvent = "PreToolUse"
	HookEventPostToolUse        HookEvent = "PostToolUse"
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookEventNotification       HookEvent = "Notification"
	HookEventUserPromptSubmit   HookEvent = "UserPromptSubmit"
	HookEventSessionStart       HookEvent = "SessionStart"
	HookEventSessionEnd         HookEvent = "SessionEnd"
	HookEventStop               HookEvent = "Stop"
	HookEventSubagentStart      HookEvent = "SubagentStart"
	HookEventSubagentStop       HookEvent = "SubagentStop"
	HookEventPreCompact         HookEvent = "PreCompact"
	HookEventPermissionRequest  HookEvent = "PermissionRequest"
	HookEventSetup              HookEvent = "Setup"
	HookEventTeammateIdle       HookEvent = "TeammateIdle"
	HookEventTaskCompleted      HookEvent = "TaskCompleted"
	HookEventConfigChange       HookEvent = "ConfigChange"
	HookEventWorktreeCreate     HookEvent = "WorktreeCreate"
	HookEventWorktreeRemove     HookEvent = "WorktreeRemove"
)

// HookEvents enumerates all known hook events.
var HookEvents = []HookEvent{
	HookEventPreToolUse,
	HookEventPostToolUse,
	HookEventPostToolUseFailure,
	HookEventNotification,
	HookEventUserPromptSubmit,
	HookEventSessionStart,
	HookEventSessionEnd,
	HookEventStop,
	HookEventSubagentStart,
	HookEventSubagentStop,
	HookEventPreCompact,
	HookEventPermissionRequest,
	HookEventSetup,
	HookEventTeammateIdle,
	HookEventTaskCompleted,
	HookEventConfigChange,
	HookEventWorktreeCreate,
	HookEventWorktreeRemove,
}

// ExitReasons enumerates all known session exit reasons.
var ExitReasons = []ExitReason{
	ExitReasonClear,
	ExitReasonLogout,
	ExitReasonPromptInputExit,
	ExitReasonOther,
	ExitReasonBypassPermissionsDisabled,
}

// SettingSource defines which filesystem settings are loaded.
type SettingSource string

const (
	SettingSourceUser    SettingSource = "user"
	SettingSourceProject SettingSource = "project"
	SettingSourceLocal   SettingSource = "local"
)

// HookCallback receives hook input payload and optional tool use ID.
// Return value is sent back to CLI as hook callback response payload.
type HookCallback func(ctx context.Context, input json.RawMessage, toolUseID *string) (any, error)

// HookCallbackMatcher routes callbacks by matcher and timeout.
type HookCallbackMatcher struct {
	Matcher string
	Hooks   []HookCallback
	Timeout *int
}

// PermissionRuleValue describes a permission rule item.
type PermissionRuleValue struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionUpdate suggests adding permission rules.
type PermissionUpdate struct {
	Type        string                `json:"type"`
	Rules       []PermissionRuleValue `json:"rules,omitempty"`
	Destination string                `json:"destination,omitempty"`
}

// CanUseToolRequest is sent when CLI requests a permission decision.
type CanUseToolRequest struct {
	ToolName              string
	Input                 map[string]any
	PermissionSuggestions []PermissionUpdate
	BlockedPath           string
	DecisionReason        string
	ToolUseID             string
	AgentID               *string
}

// PermissionResult is returned by CanUseTool callback.
type PermissionResult struct {
	Behavior           string             `json:"behavior"`
	UpdatedInput       map[string]any     `json:"updatedInput,omitempty"`
	UpdatedPermissions []PermissionUpdate `json:"updatedPermissions,omitempty"`
	Message            string             `json:"message,omitempty"`
	Interrupt          bool               `json:"interrupt,omitempty"`
	ToolUseID          string             `json:"toolUseID,omitempty"`
}

// CanUseTool decides if a tool call should be allowed.
type CanUseTool func(ctx context.Context, req CanUseToolRequest) (PermissionResult, error)

// IncomingControlRequest represents control requests initiated by the CLI process.
type IncomingControlRequest struct {
	RequestID string
	Subtype   string
	Raw       json.RawMessage
}

// ControlRequestHandler handles incoming control requests from the CLI process.
// Return value is encoded as the "response" field in a success control_response.
type ControlRequestHandler func(ctx context.Context, req IncomingControlRequest) (any, error)

// MCPMessageRequest represents SDK-side handling of mcp_message control requests.
type MCPMessageRequest struct {
	ServerName string
	Message    json.RawMessage
}

// OnMCPMessage handles incoming mcp_message control requests for SDK-managed servers.
type OnMCPMessage func(ctx context.Context, req MCPMessageRequest) (json.RawMessage, error)

// AgentDefinition defines a custom subagent for initialize payload.
type AgentDefinition struct {
	Description                        string          `json:"description"`
	Tools                              []string        `json:"tools,omitempty"`
	DisallowedTools                    []string        `json:"disallowedTools,omitempty"`
	Prompt                             string          `json:"prompt"`
	Model                              string          `json:"model,omitempty"`
	MCPServers                         json.RawMessage `json:"mcpServers,omitempty"`
	CriticalSystemReminderExperimental string          `json:"criticalSystemReminder_EXPERIMENTAL,omitempty"`
	Skills                             []string        `json:"skills,omitempty"`
	MaxTurns                           *int            `json:"maxTurns,omitempty"`
}

// MCPStdioServerConfig configures a stdio MCP server.
type MCPStdioServerConfig struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPSSEServerConfig configures an SSE MCP server.
type MCPSSEServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPHTTPServerConfig configures an HTTP MCP server.
type MCPHTTPServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPServerConfigForProcessTransport is the wire config used by CLI.
type MCPServerConfigForProcessTransport = map[string]any

// PluginConfig is currently only local plugin path.
type PluginConfig struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// ThinkingMode controls reasoning behavior.
type ThinkingMode string

const (
	ThinkingAdaptive ThinkingMode = "adaptive"
	ThinkingEnabled  ThinkingMode = "enabled"
	ThinkingDisabled ThinkingMode = "disabled"
)

// ThinkingConfig maps to SDK thinking option.
type ThinkingConfig struct {
	Type         ThinkingMode `json:"type"`
	BudgetTokens *int         `json:"budgetTokens,omitempty"`
}

// SandboxSettings is passed through to CLI via --settings.
type SandboxSettings = map[string]any

// Options configures process and session behavior.
type Options struct {
	// Command is the executable used to start Claude Code. Defaults to "claude".
	Command string
	// PathToClaudeCodeExecutable overrides Command and points directly to executable path.
	PathToClaudeCodeExecutable string
	// CWD sets the working directory used by the spawned process.
	CWD string
	// Env replaces the process environment when set. If nil, os.Environ() is used.
	Env []string
	// EnvMap overlays environment variables on top of current process env.
	EnvMap map[string]string
	// Stderr receives stderr output from the spawned process.
	Stderr io.Writer

	// ExtraArgs are appended to generated CLI arguments as-is.
	ExtraArgs []string
	// ExtraFlagArgs maps --flag to value. nil value writes flag-only.
	ExtraFlagArgs map[string]*string

	Agent                 string
	Agents                map[string]AgentDefinition
	AllowedTools          []string
	DisallowedTools       []string
	AdditionalDirectories []string
	Tools                 []string

	Model         string
	FallbackModel string
	Betas         []string

	ContinueConversation bool
	Resume               string
	SessionID            string
	ResumeSessionAt      string
	ForkSession          bool
	PersistSession       *bool

	PermissionMode                  PermissionMode
	AllowDangerouslySkipPermissions bool
	PermissionPromptToolName        string

	IncludePartialMessages  bool
	EnableFileCheckpointing bool
	MaxTurns                *int
	MaxBudgetUSD            *float64
	Effort                  string
	Thinking                *ThinkingConfig
	MaxThinkingTokens       *int

	MCPServers      map[string]MCPServerConfigForProcessTransport
	StrictMCPConfig bool
	Plugins         []PluginConfig
	SettingSources  []SettingSource

	Debug     bool
	DebugFile string

	Sandbox SandboxSettings

	SystemPrompt       string
	AppendSystemPrompt string
	PromptSuggestions  *bool
	JSONSchema         json.RawMessage

	CanUseTool       CanUseTool
	Hooks            map[HookEvent][]HookCallbackMatcher
	OnMCPMessage     OnMCPMessage
	OnControlRequest ControlRequestHandler
}
