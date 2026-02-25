package loop

import (
	"context"
	"time"

	"github.com/opus-domini/praetor/internal/providers"
)

// Agent identifies an execution agent used by the loop runner.
type Agent string

const (
	AgentClaude Agent = Agent(providers.Claude)
	AgentCodex  Agent = Agent(providers.Codex)
	AgentNone   Agent = "none"
)

var validExecutors = map[Agent]struct{}{
	AgentClaude: {},
	AgentCodex:  {},
}

var validReviewers = map[Agent]struct{}{
	AgentClaude: {},
	AgentCodex:  {},
	AgentNone:   {},
}

// Plan describes an immutable execution plan.
type Plan struct {
	Schema string `json:"$schema,omitempty"`
	Title  string `json:"title,omitempty"`
	Tasks  []Task `json:"tasks"`
}

// Task is one plan task definition.
type Task struct {
	ID          string   `json:"id,omitempty"`
	Title       string   `json:"title"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Executor    Agent    `json:"executor,omitempty"`
	Reviewer    Agent    `json:"reviewer,omitempty"`
	Model       string   `json:"model,omitempty"`
	Description string   `json:"description,omitempty"`
	Criteria    string   `json:"criteria,omitempty"`
}

// TaskStatus tracks mutable execution status for a task.
type TaskStatus string

const (
	TaskStatusOpen TaskStatus = "open"
	TaskStatusDone TaskStatus = "done"
)

// StateTask is one mutable state entry for a task.
type StateTask struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	Executor    Agent      `json:"executor,omitempty"`
	Reviewer    Agent      `json:"reviewer,omitempty"`
	Model       string     `json:"model,omitempty"`
	Description string     `json:"description,omitempty"`
	Criteria    string     `json:"criteria,omitempty"`
	Status      TaskStatus `json:"status"`
}

// State stores mutable progress for one plan file.
type State struct {
	PlanFile     string      `json:"plan_file"`
	PlanChecksum string      `json:"plan_checksum"`
	CreatedAt    string      `json:"created_at"`
	UpdatedAt    string      `json:"updated_at"`
	Tasks        []StateTask `json:"tasks"`
}

// DoneCount returns how many tasks are completed.
func (s State) DoneCount() int {
	done := 0
	for _, task := range s.Tasks {
		if task.Status == TaskStatusDone {
			done++
		}
	}
	return done
}

// OpenCount returns how many tasks are still open.
func (s State) OpenCount() int {
	return len(s.Tasks) - s.DoneCount()
}

// RunnerOptions controls loop execution behavior.
type RunnerOptions struct {
	StateRoot       string
	Workdir         string
	DefaultExecutor Agent
	DefaultReviewer Agent
	MaxRetries      int
	MaxIterations   int
	SkipReview      bool
	Force           bool
	CodexBin        string
	ClaudeBin       string
	TMUXSession     string
	Verbose         bool
	NoColor         bool
	Isolation       IsolationMode
	PostTaskHook    string
}

// IsolationMode controls how tasks are isolated from the main working tree.
type IsolationMode string

const (
	IsolationWorktree IsolationMode = "worktree"
	IsolationOff      IsolationMode = "off"
)

// RunnerStats summarizes one run invocation.
type RunnerStats struct {
	PlanFile      string
	StateFile     string
	Iterations    int
	TasksDone     int
	TasksRejected int
	TotalCostUSD  float64
	TotalDuration time.Duration
}

// AgentRequest is one execution request for an agent runtime.
type AgentRequest struct {
	Role         string
	Agent        Agent
	Prompt       string
	SystemPrompt string
	Model        string
	Workdir      string
	RunDir       string
	OutputPrefix string
	TaskLabel    string
	CodexBin     string
	ClaudeBin    string
	Verbose      bool
}

// AgentResult holds output and metrics from one agent invocation.
type AgentResult struct {
	Output    string
	CostUSD   float64
	DurationS float64
}

// AgentRuntime executes prompts on a provider backend.
type AgentRuntime interface {
	Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}
