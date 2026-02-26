package agents

import (
	"context"
	"encoding/json"
	"strings"
)

// ID identifies one cognitive agent backend.
type ID string

const (
	None   ID = "none"
	Codex  ID = "codex"
	Claude ID = "claude"
	Gemini ID = "gemini"
	Ollama ID = "ollama"
)

// Transport identifies how the backend is reached.
type Transport string

const (
	TransportCLI  Transport = "cli"
	TransportREST Transport = "rest"
)

// Capabilities describes supported cognitive phases and runtime behavior.
type Capabilities struct {
	Transport        Transport
	SupportsPlan     bool
	SupportsExecute  bool
	SupportsReview   bool
	RequiresTTY      bool
	StructuredOutput bool
}

// PlanRequest is the macro-planning request contract.
type PlanRequest struct {
	Objective        string
	WorkspaceContext string
	Workdir          string
	Model            string
	RunDir           string
	OutputPrefix     string
	TaskLabel        string
}

// PlanResponse is the standardized planning result.
type PlanResponse struct {
	Manifest  json.RawMessage
	Output    string
	Model     string
	CostUSD   float64
	DurationS float64
	Strategy  string
}

// ExecuteRequest is the atomic execution request contract.
type ExecuteRequest struct {
	Prompt       string
	SystemPrompt string
	Workdir      string
	Model        string
	RunDir       string
	OutputPrefix string
	TaskLabel    string
	OneShot      bool // true = one-shot terminal (praetor exec); false = streaming/PTY (plan pipeline)
}

// ExecuteResponse is the standardized execution result.
type ExecuteResponse struct {
	Output    string
	Model     string
	CostUSD   float64
	DurationS float64
	Strategy  string
}

// ReviewRequest is the isolated review request contract.
type ReviewRequest struct {
	Prompt       string
	SystemPrompt string
	Workdir      string
	Model        string
	RunDir       string
	OutputPrefix string
	TaskLabel    string
}

// ReviewDecision classifies review output.
type ReviewDecision string

const (
	DecisionUnknown ReviewDecision = "unknown"
	DecisionPass    ReviewDecision = "pass"
	DecisionFail    ReviewDecision = "fail"
)

// ReviewResponse is the standardized review result.
type ReviewResponse struct {
	Decision  ReviewDecision
	Reason    string
	Output    string
	CostUSD   float64
	DurationS float64
	Strategy  string
}

// Agent is the central polymorphic contract across CLI and REST providers.
type Agent interface {
	ID() ID
	Capabilities() Capabilities
	Plan(ctx context.Context, req PlanRequest) (PlanResponse, error)
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error)
	Review(ctx context.Context, req ReviewRequest) (ReviewResponse, error)
}

// Normalize canonicalizes agent identifiers for validation and routing.
func Normalize(raw string) ID {
	return ID(strings.ToLower(strings.TrimSpace(raw)))
}

// IsSupported reports whether id maps to a built-in agent.
func IsSupported(id ID) bool {
	switch id {
	case Codex, Claude, Gemini, Ollama:
		return true
	default:
		return false
	}
}
