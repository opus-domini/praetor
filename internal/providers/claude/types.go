package claude

import "encoding/json"

// SDKMessage is a raw stream event emitted by Claude Code.
type SDKMessage struct {
	Type string
	Raw  json.RawMessage
}

// AssistantMessage is the "assistant" message from the model.
type AssistantMessage struct {
	Type            string          `json:"type"`
	UUID            string          `json:"uuid"`
	SessionID       string          `json:"session_id"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	Message         json.RawMessage `json:"message"`
}

// SystemInitMessage is the first "system" subtype:"init" message.
type SystemInitMessage struct {
	Type           string   `json:"type"`
	Subtype        string   `json:"subtype"`
	Model          string   `json:"model"`
	CWD            string   `json:"cwd"`
	Tools          []string `json:"tools"`
	PermissionMode string   `json:"permissionMode"`
	ClaudeVersion  string   `json:"claude_code_version"`
	UUID           string   `json:"uuid"`
	SessionID      string   `json:"session_id"`
}

// StatusMessage is the "system" subtype:"status" message.
type StatusMessage struct {
	Type           string `json:"type"`
	Subtype        string `json:"subtype"`
	Status         string `json:"status"`
	PermissionMode string `json:"permissionMode"`
}

// NonNullableUsage holds token usage data from a result message.
type NonNullableUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// ModelUsage holds per-model usage breakdown.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	WebSearchRequests        int     `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int     `json:"contextWindow"`
	MaxOutputTokens          int     `json:"maxOutputTokens"`
}

// PermissionDenial records a denied tool use attempt.
type PermissionDenial struct {
	ToolName  string         `json:"tool_name"`
	ToolUseID string         `json:"tool_use_id"`
	ToolInput map[string]any `json:"tool_input"`
}

// ResultMessage decodes the common "result" message shape.
type ResultMessage struct {
	Type              string                `json:"type"`
	Subtype           string                `json:"subtype"`
	IsError           bool                  `json:"is_error"`
	Result            string                `json:"result"`
	SessionID         string                `json:"session_id"`
	UUID              string                `json:"uuid"`
	DurationMS        int                   `json:"duration_ms"`
	DurationAPIMS     int                   `json:"duration_api_ms"`
	NumTurns          int                   `json:"num_turns"`
	StopReason        *string               `json:"stop_reason"`
	TotalCostUSD      float64               `json:"total_cost_usd"`
	Usage             *NonNullableUsage     `json:"usage,omitempty"`
	ModelUsage        map[string]ModelUsage `json:"modelUsage,omitempty"`
	PermissionDenials []PermissionDenial    `json:"permission_denials,omitempty"`
	StructuredOutput  json.RawMessage       `json:"structured_output,omitempty"`
	Errors            []string              `json:"errors,omitempty"`
	Raw               json.RawMessage       `json:"-"`
}

// UserTextMessage is the SDK user message envelope used for text prompts.
type UserTextMessage struct {
	Type            string      `json:"type"`
	SessionID       string      `json:"session_id"`
	Message         UserMessage `json:"message"`
	ParentToolUseID *string     `json:"parent_tool_use_id"`
}

// UserMessage is the inner user message payload.
type UserMessage struct {
	Role    string            `json:"role"`
	Content []UserTextContent `json:"content"`
}

// UserTextContent is a text content block.
type UserTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SlashCommand is returned by initialization command list.
type SlashCommand struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ArgumentHint string `json:"argumentHint"`
}

// ModelInfo describes supported model metadata.
type ModelInfo struct {
	Value                    string   `json:"value"`
	DisplayName              string   `json:"displayName"`
	Description              string   `json:"description"`
	SupportsEffort           *bool    `json:"supportsEffort,omitempty"`
	SupportedEffortLevels    []string `json:"supportedEffortLevels,omitempty"`
	SupportsAdaptiveThinking *bool    `json:"supportsAdaptiveThinking,omitempty"`
}

// AccountInfo is returned during initialization.
type AccountInfo struct {
	Email            string `json:"email,omitempty"`
	Organization     string `json:"organization,omitempty"`
	SubscriptionType string `json:"subscriptionType,omitempty"`
	TokenSource      string `json:"tokenSource,omitempty"`
	APIKeySource     string `json:"apiKeySource,omitempty"`
}

// InitializeResponse is the control response payload for initialize.
type InitializeResponse struct {
	Commands              []SlashCommand `json:"commands"`
	OutputStyle           string         `json:"output_style"`
	AvailableOutputStyles []string       `json:"available_output_styles"`
	Models                []ModelInfo    `json:"models"`
	Account               AccountInfo    `json:"account"`
}

// MCPSupportServerInfo represents server metadata in status.
type MCPSupportServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerToolInfo describes tools exposed by an MCP server.
type MCPServerToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Annotations struct {
		ReadOnly    *bool `json:"readOnly,omitempty"`
		Destructive *bool `json:"destructive,omitempty"`
		OpenWorld   *bool `json:"openWorld,omitempty"`
	} `json:"annotations,omitempty"`
}

// MCPServerStatus describes current connection state.
type MCPServerStatus struct {
	Name       string                `json:"name"`
	Status     string                `json:"status"`
	ServerInfo *MCPSupportServerInfo `json:"serverInfo,omitempty"`
	Error      string                `json:"error,omitempty"`
	Config     json.RawMessage       `json:"config,omitempty"`
	Scope      string                `json:"scope,omitempty"`
	Tools      []MCPServerToolInfo   `json:"tools,omitempty"`
}

// MCPSetServersResult is returned by mcp_set_servers.
type MCPSetServersResult struct {
	Added   []string          `json:"added"`
	Removed []string          `json:"removed"`
	Errors  map[string]string `json:"errors"`
}

type controlRequestEnvelope struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Request   any    `json:"request"`
}

type incomingControlRequestEnvelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
}

type incomingControlCancelRequestEnvelope struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

type controlResponseEnvelope struct {
	Type     string          `json:"type"`
	Response controlResponse `json:"response"`
}

type controlResponse struct {
	Subtype                   string                           `json:"subtype"`
	RequestID                 string                           `json:"request_id"`
	Response                  json.RawMessage                  `json:"response,omitempty"`
	Error                     string                           `json:"error,omitempty"`
	PendingPermissionRequests []incomingControlRequestEnvelope `json:"pending_permission_requests,omitempty"`
}

type outgoingControlResponseEnvelope struct {
	Type     string                  `json:"type"`
	Response outgoingControlResponse `json:"response"`
}

type outgoingControlResponse struct {
	Subtype   string `json:"subtype"`
	RequestID string `json:"request_id"`
	Response  any    `json:"response,omitempty"`
	Error     string `json:"error,omitempty"`
}

type initializeRequest struct {
	Subtype            string                                 `json:"subtype"`
	Hooks              map[HookEvent][]sdkHookCallbackMatcher `json:"hooks,omitempty"`
	SDKMCPServers      []string                               `json:"sdkMcpServers,omitempty"`
	JSONSchema         json.RawMessage                        `json:"jsonSchema,omitempty"`
	SystemPrompt       string                                 `json:"systemPrompt,omitempty"`
	AppendSystemPrompt string                                 `json:"appendSystemPrompt,omitempty"`
	Agents             map[string]AgentDefinition             `json:"agents,omitempty"`
	PromptSuggestions  *bool                                  `json:"promptSuggestions,omitempty"`
}

type sdkHookCallbackMatcher struct {
	Matcher         string   `json:"matcher,omitempty"`
	HookCallbackIDs []string `json:"hookCallbackIds"`
	Timeout         *int     `json:"timeout,omitempty"`
}

type requestSubtypeOnly struct {
	Subtype string `json:"subtype"`
}

type canUseToolControlRequest struct {
	Subtype               string             `json:"subtype"`
	ToolName              string             `json:"tool_name"`
	Input                 map[string]any     `json:"input"`
	PermissionSuggestions []PermissionUpdate `json:"permission_suggestions,omitempty"`
	BlockedPath           string             `json:"blocked_path,omitempty"`
	DecisionReason        string             `json:"decision_reason,omitempty"`
	ToolUseID             string             `json:"tool_use_id"`
	AgentID               *string            `json:"agent_id,omitempty"`
}

type hookCallbackControlRequest struct {
	Subtype    string          `json:"subtype"`
	CallbackID string          `json:"callback_id"`
	Input      json.RawMessage `json:"input"`
	ToolUseID  *string         `json:"tool_use_id,omitempty"`
}

type mcpMessageControlRequest struct {
	Subtype    string          `json:"subtype"`
	ServerName string          `json:"server_name"`
	Message    json.RawMessage `json:"message"`
}

// RewindFilesResult is returned by RewindFiles.
type RewindFilesResult struct {
	CanRewind    bool     `json:"canRewind"`
	Error        string   `json:"error,omitempty"`
	FilesChanged []string `json:"filesChanged,omitempty"`
	Insertions   int      `json:"insertions,omitempty"`
	Deletions    int      `json:"deletions,omitempty"`
}
