package domain

import (
	"context"
	"strings"
	"time"
)

// Agent identifies an execution agent used by the loop runner.
type Agent string

const (
	AgentClaude     Agent = "claude"
	AgentCodex      Agent = "codex"
	AgentCopilot    Agent = "copilot"
	AgentGemini     Agent = "gemini"
	AgentKimi       Agent = "kimi"
	AgentOpenCode   Agent = "opencode"
	AgentOpenRouter Agent = "openrouter"
	AgentOllama     Agent = "ollama"
	AgentNone       Agent = "none"
)

// ValidExecutors lists agents that may execute tasks.
var ValidExecutors = map[Agent]struct{}{
	AgentClaude:     {},
	AgentCodex:      {},
	AgentCopilot:    {},
	AgentGemini:     {},
	AgentKimi:       {},
	AgentOpenCode:   {},
	AgentOpenRouter: {},
	AgentOllama:     {},
}

// ValidReviewers lists agents that may review tasks.
var ValidReviewers = map[Agent]struct{}{
	AgentClaude:     {},
	AgentCodex:      {},
	AgentCopilot:    {},
	AgentGemini:     {},
	AgentKimi:       {},
	AgentOpenCode:   {},
	AgentOpenRouter: {},
	AgentOllama:     {},
	AgentNone:       {},
}

// NormalizeAgent lowercases and trims an agent identifier.
func NormalizeAgent(agent Agent) Agent {
	return Agent(strings.ToLower(strings.TrimSpace(string(agent))))
}

// Plan describes an immutable execution plan.
type Plan struct {
	Name      string         `json:"name"`
	Summary   string         `json:"summary,omitempty"`
	Meta      PlanMeta       `json:"meta,omitempty"`
	Settings  PlanSettings   `json:"settings"`
	Quality   PlanQuality    `json:"quality,omitempty"`
	Cognitive *PlanCognitive `json:"cognitive,omitempty"`
	Tasks     []Task         `json:"tasks"`
}

type PlanMeta struct {
	Source    string        `json:"source,omitempty"`
	CreatedAt string        `json:"created_at,omitempty"`
	CreatedBy string        `json:"created_by,omitempty"`
	Generator PlanGenerator `json:"generator,omitempty"`
}

type PlanGenerator struct {
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	PromptHash string `json:"prompt_hash,omitempty"`
}

type PlanSettings struct {
	Agents          PlanAgents      `json:"agents"`
	ExecutionPolicy ExecutionPolicy `json:"execution_policy,omitempty"`
}

type PlanAgents struct {
	Planner  PlanAgentConfig `json:"planner,omitempty"`
	Executor PlanAgentConfig `json:"executor"`
	Reviewer PlanAgentConfig `json:"reviewer"`
}

type PlanAgentConfig struct {
	Agent Agent  `json:"agent"`
	Model string `json:"model,omitempty"`
}

type ExecutionPolicy struct {
	MaxTotalIterations int          `json:"max_total_iterations,omitempty"`
	MaxRetriesPerTask  int          `json:"max_retries_per_task,omitempty"`
	Timeout            string       `json:"timeout,omitempty"`
	Budget             BudgetPolicy `json:"budget,omitempty"`
	StallDetection     StallPolicy  `json:"stall_detection,omitempty"`
}

type BudgetPolicy struct {
	Execute int `json:"execute,omitempty"`
	Review  int `json:"review,omitempty"`
}

type StallPolicy struct {
	Enabled   bool    `json:"enabled,omitempty"`
	Window    int     `json:"window,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
}

type PlanQuality struct {
	EvidenceFormat string   `json:"evidence_format,omitempty"`
	Required       []string `json:"required,omitempty"`
	Optional       []string `json:"optional,omitempty"`
}

// PlanCognitive captures cognitive metadata for a plan.
type PlanCognitive struct {
	Assumptions   []string `json:"assumptions,omitempty"`
	OpenQuestions []string `json:"open_questions,omitempty"`
	FailureModes  []string `json:"failure_modes,omitempty"`
	Decisions     []string `json:"decisions,omitempty"`
}

// Task is one plan task definition.
type Task struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	DependsOn   []string         `json:"depends_on,omitempty"`
	Description string           `json:"description,omitempty"`
	Acceptance  []string         `json:"acceptance"`
	Constraints *TaskConstraints `json:"constraints,omitempty"`
	Agents      *TaskAgents      `json:"agents,omitempty"`
}

// TaskConstraints defines per-task execution restrictions.
type TaskConstraints struct {
	AllowedTools []string `json:"allowed_tools,omitempty"`
	DeniedTools  []string `json:"denied_tools,omitempty"`
	Timeout      string   `json:"timeout,omitempty"`
}

// TaskAgents allows per-task agent executor/reviewer overrides.
type TaskAgents struct {
	Executor      string `json:"executor,omitempty"`
	Reviewer      string `json:"reviewer,omitempty"`
	ExecutorModel string `json:"executor_model,omitempty"`
	ReviewerModel string `json:"reviewer_model,omitempty"`
}

// TaskStatus tracks mutable execution status for a task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskExecuting TaskStatus = "executing"
	TaskReviewing TaskStatus = "reviewing"
	TaskDone      TaskStatus = "done"
	TaskFailed    TaskStatus = "failed"
)

// StateTask is one mutable state entry for a task.
type StateTask struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	Description string     `json:"description,omitempty"`
	Acceptance  []string   `json:"acceptance,omitempty"`
	Status      TaskStatus `json:"status"`
	Attempt     int        `json:"attempt,omitempty"`
	Feedback    string     `json:"feedback,omitempty"`
}

// State stores mutable progress for one plan.
type State struct {
	PlanSlug     string      `json:"plan_slug"`
	PlanChecksum string      `json:"plan_checksum"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
	Outcome      RunOutcome  `json:"outcome,omitempty"`
	Tasks        []StateTask `json:"tasks"`
}

// DoneCount returns how many tasks are completed.
func (s State) DoneCount() int {
	n := 0
	for _, task := range s.Tasks {
		if task.Status == TaskDone {
			n++
		}
	}
	return n
}

// FailedCount returns how many tasks exhausted retries.
func (s State) FailedCount() int {
	n := 0
	for _, task := range s.Tasks {
		if task.Status == TaskFailed {
			n++
		}
	}
	return n
}

// ActiveCount returns how many tasks are not in a terminal state.
func (s State) ActiveCount() int {
	return len(s.Tasks) - s.DoneCount() - s.FailedCount()
}

// RunnerOptions controls loop execution behavior.
type RunnerOptions struct {
	ProjectHome         string
	Workdir             string
	RunnerMode          RunnerMode
	DefaultExecutor     Agent
	DefaultReviewer     Agent
	ExecutorModel       string
	ReviewerModel       string
	PlannerAgent        Agent
	PlannerModel        string
	Objective           string
	MaxRetries          int
	MaxIterations       int
	MaxTransitions      int
	KeepLastRuns        int
	Timeout             time.Duration
	BudgetExecute       int
	BudgetReview        int
	StallDetection      bool
	StallWindow         int
	StallThreshold      float64
	PlannerAgentSet     bool
	PlannerModelSet     bool
	ExecutorAgentSet    bool
	ExecutorModelSet    bool
	ReviewerAgentSet    bool
	ReviewerModelSet    bool
	MaxRetriesSet       bool
	MaxIterationsSet    bool
	TimeoutSet          bool
	BudgetExecuteSet    bool
	BudgetReviewSet     bool
	StallDetectionSet   bool
	StallWindowSet      bool
	StallThresholdSet   bool
	SkipReview          bool
	Force               bool
	CodexBin            string
	ClaudeBin           string
	CopilotBin          string
	GeminiBin           string
	KimiBin             string
	OpenCodeBin         string
	OpenRouterURL       string
	OpenRouterModel     string
	OpenRouterKeyEnv    string
	OllamaURL           string
	OllamaModel         string
	TMUXSession         string
	Verbose             bool
	NoColor             bool
	Isolation           IsolationMode
	PostTaskHook        string
	FallbackAgent       Agent
	FallbackOnTransient Agent
	FallbackOnAuth      Agent
}

// RunnerMode controls how external agent commands are executed.
type RunnerMode string

const (
	RunnerTMUX   RunnerMode = "tmux"
	RunnerPTY    RunnerMode = "pty"
	RunnerDirect RunnerMode = "direct"
)

// IsolationMode controls how tasks are isolated from the main working tree.
type IsolationMode string

const (
	IsolationWorktree IsolationMode = "worktree"
	IsolationOff      IsolationMode = "off"
)

// RunnerStats summarizes one run invocation.
type RunnerStats struct {
	PlanSlug      string
	StateFile     string
	Outcome       RunOutcome
	Iterations    int
	TasksDone     int
	TasksRejected int
	TotalCostUSD  float64
	TotalDuration time.Duration
}

type RunOutcome string

const (
	RunSuccess  RunOutcome = "success"
	RunPartial  RunOutcome = "partial"
	RunFailed   RunOutcome = "failed"
	RunCanceled RunOutcome = "canceled"
)

// AgentRequest is one execution request for an agent runtime.
type AgentRequest struct {
	Role             string
	Agent            Agent
	Prompt           string
	SystemPrompt     string
	Model            string
	Workdir          string
	RunDir           string
	OutputPrefix     string
	TaskLabel        string
	CodexBin         string
	ClaudeBin        string
	CopilotBin       string
	GeminiBin        string
	KimiBin          string
	OpenCodeBin      string
	OpenRouterURL    string
	OpenRouterModel  string
	OpenRouterKeyEnv string
	Verbose          bool
}

// AgentResult holds output and metrics from one agent invocation.
type AgentResult struct {
	Output    string
	CostUSD   float64
	DurationS float64
	Strategy  ExecutionStrategy
}

// ExecutionStrategy captures which runtime path was used to execute an agent call.
type ExecutionStrategy string

const (
	ExecutionStrategyStructured ExecutionStrategy = "structured"
	ExecutionStrategyProcess    ExecutionStrategy = "process"
	ExecutionStrategyPTY        ExecutionStrategy = "pty"
)

// CommandSpec describes a process invocation, agnostic of how it will be executed.
type CommandSpec struct {
	Args       []string // Full command: ["codex", "exec", "--json", ...]
	Env        []string // Additional environment variables (KEY=VALUE)
	Dir        string   // Working directory
	Stdin      string   // Content to write to stdin ("" = no stdin)
	WindowHint string   // Hint for tmux window naming (e.g. task label); ignored by non-tmux runners
}

// ProcessResult holds the raw output of a completed process.
type ProcessResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// AgentSpec knows how to build a CLI invocation for one agent
// and how to interpret its output.
type AgentSpec interface {
	// BuildCommand produces the command-line invocation for this agent.
	BuildCommand(req AgentRequest) (CommandSpec, error)

	// ParseOutput interprets the agent's stdout and extracts
	// the usable output text and cost (if available).
	ParseOutput(stdout string) (output string, cost float64, err error)
}

// ProcessRunner executes a CommandSpec and returns raw process output.
// The implementation decides how to run it (tmux, direct, etc.).
type ProcessRunner interface {
	Run(ctx context.Context, spec CommandSpec, runDir, prefix string) (ProcessResult, error)
}

// SessionManager is optionally implemented by runners that manage sessions.
type SessionManager interface {
	EnsureSession() error
	Cleanup()
	SessionName() string
}

// AgentRuntime executes prompts on a provider backend.
type AgentRuntime interface {
	Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}

// CostEntry records one agent invocation's metrics.
type CostEntry struct {
	Timestamp string
	RunID     string
	TaskID    string
	Agent     string
	Role      string
	DurationS float64
	Status    string
	CostUSD   float64
}

// CheckpointEntry records one state transition in the audit log.
type CheckpointEntry struct {
	Timestamp string
	Status    string
	TaskID    string
	Signature string
	RunID     string
	Message   string
}

// RenderSink is the interface for structured terminal output.
// It decouples the orchestration pipeline from the concrete renderer implementation.
type RenderSink interface {
	Header(title string)
	KV(label, value string)
	Task(progress, label, title string)
	Phase(phase, agent, detail string)
	Info(message string)
	Success(message string)
	Warn(message string)
	Error(message string)
	Summary(done, rejected, iterations int, totalCostUSD float64, totalDuration time.Duration)
}

// PlanStatus describes execution status of a plan.
type PlanStatus struct {
	PlanSlug  string
	StateFile string
	UpdatedAt string
	Outcome   RunOutcome
	Done      int
	Failed    int
	Active    int
	Total     int
	Running   bool
	Tasks     []StateTask
}
