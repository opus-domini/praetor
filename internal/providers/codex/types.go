package codex

import "encoding/json"

// ApprovalMode controls permission prompts/policy used by Codex.
type ApprovalMode string

const (
	ApprovalModeNever     ApprovalMode = "never"
	ApprovalModeOnRequest ApprovalMode = "on-request"
	ApprovalModeOnFailure ApprovalMode = "on-failure"
	ApprovalModeUntrusted ApprovalMode = "untrusted"
)

// SandboxMode controls CLI sandboxing.
type SandboxMode string

const (
	SandboxModeReadOnly         SandboxMode = "read-only"
	SandboxModeWorkspaceWrite   SandboxMode = "workspace-write"
	SandboxModeDangerFullAccess SandboxMode = "danger-full-access"
)

// ModelReasoningEffort controls model effort settings.
type ModelReasoningEffort string

const (
	ModelReasoningEffortMinimal ModelReasoningEffort = "minimal"
	ModelReasoningEffortLow     ModelReasoningEffort = "low"
	ModelReasoningEffortMedium  ModelReasoningEffort = "medium"
	ModelReasoningEffortHigh    ModelReasoningEffort = "high"
	ModelReasoningEffortXHigh   ModelReasoningEffort = "xhigh"
)

// WebSearchMode configures search behavior.
type WebSearchMode string

const (
	WebSearchModeDisabled WebSearchMode = "disabled"
	WebSearchModeCached   WebSearchMode = "cached"
	WebSearchModeLive     WebSearchMode = "live"
)

// UserInputType represents structured input entries accepted by the SDK.
type UserInputType string

const (
	UserInputTypeText       UserInputType = "text"
	UserInputTypeLocalImage UserInputType = "local_image"
)

// UserInput is one structured input block.
// Supported forms:
// - {Type: "text", Text: "..."}
// - {Type: "local_image", Path: "..."}
type UserInput struct {
	Type UserInputType `json:"type"`
	Text string        `json:"text,omitempty"`
	Path string        `json:"path,omitempty"`
}

// Usage is token usage returned by turn.completed.
type Usage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

// ThreadError is an error payload emitted by the stream.
type ThreadError struct {
	Message string `json:"message"`
}

// FileUpdateChange describes one changed file in a patch.
type FileUpdateChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// TodoItem is one entry in the agent todo list.
type TodoItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// McpToolCallError describes a tool-call failure payload.
type McpToolCallError struct {
	Message string `json:"message"`
}

// McpToolCallResult describes a successful MCP tool-call result payload.
type McpToolCallResult struct {
	Content           any `json:"content"`
	StructuredContent any `json:"structured_content"`
}

// ThreadItem is the canonical SDK item payload.
// It keeps commonly-used fields decoded while preserving full raw JSON.
type ThreadItem struct {
	ID               string             `json:"id,omitempty"`
	Type             string             `json:"type"`
	Text             string             `json:"text,omitempty"`
	Message          string             `json:"message,omitempty"`
	Query            string             `json:"query,omitempty"`
	Command          string             `json:"command,omitempty"`
	AggregatedOutput string             `json:"aggregated_output,omitempty"`
	ExitCode         *int               `json:"exit_code,omitempty"`
	Status           string             `json:"status,omitempty"`
	Server           string             `json:"server,omitempty"`
	Tool             string             `json:"tool,omitempty"`
	Arguments        any                `json:"arguments,omitempty"`
	Result           *McpToolCallResult `json:"result,omitempty"`
	Error            *McpToolCallError  `json:"error,omitempty"`
	Changes          []FileUpdateChange `json:"changes,omitempty"`
	Items            []TodoItem         `json:"items,omitempty"`
	Raw              json.RawMessage    `json:"-"`
}

// ThreadEvent is one top-level JSONL event emitted by `codex exec --experimental-json`.
type ThreadEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id,omitempty"`
	Item     *ThreadItem     `json:"item,omitempty"`
	Usage    *Usage          `json:"usage,omitempty"`
	Error    *ThreadError    `json:"error,omitempty"`
	Message  string          `json:"message,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// TurnOptions controls one turn execution.
type TurnOptions struct {
	// OutputSchema is a JSON object schema that will be written to a temp file
	// and passed to Codex via --output-schema.
	OutputSchema any
}

// Turn is the completed result from Run().
type Turn struct {
	Items         []ThreadItem
	FinalResponse string
	Usage         *Usage
}

// StreamedTurn is returned by RunStreamed().
// Consume Events until channel close, then read Done for final status.
type StreamedTurn struct {
	Events <-chan ThreadEvent
	Done   <-chan error
}

// ThreadOptions defines per-thread defaults.
type ThreadOptions struct {
	Model                 string
	SandboxMode           SandboxMode
	WorkingDirectory      string
	SkipGitRepoCheck      bool
	ModelReasoningEffort  ModelReasoningEffort
	NetworkAccessEnabled  *bool
	WebSearchMode         WebSearchMode
	WebSearchEnabled      *bool
	ApprovalPolicy        ApprovalMode
	AdditionalDirectories []string
}

// CodexOptions defines client-wide settings.
type CodexOptions struct {
	// CodexPathOverride overrides automatic executable resolution.
	CodexPathOverride string
	BaseURL           string
	APIKey            string
	// Config emits repeated --config key=value CLI flags.
	Config map[string]any
	// Env, when provided, becomes the full environment for the child process.
	// Required internal vars are injected on top.
	Env map[string]string
}
