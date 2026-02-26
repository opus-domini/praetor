package loop

import "github.com/opus-domini/praetor/internal/domain"

// Type aliases — all domain types are defined in internal/domain.
// These aliases preserve backward compatibility so existing loop and cli code
// continues to compile without import path changes.

type Agent = domain.Agent
type Plan = domain.Plan
type Task = domain.Task
type TaskStatus = domain.TaskStatus
type StateTask = domain.StateTask
type State = domain.State
type RunnerOptions = domain.RunnerOptions
type RunnerMode = domain.RunnerMode
type IsolationMode = domain.IsolationMode
type RunnerStats = domain.RunnerStats
type AgentRequest = domain.AgentRequest
type AgentResult = domain.AgentResult
type CommandSpec = domain.CommandSpec
type ProcessResult = domain.ProcessResult
type CostEntry = domain.CostEntry
type CheckpointEntry = domain.CheckpointEntry
type PlanStatus = domain.PlanStatus
type ExecutorResult = domain.ExecutorResult
type ReviewDecision = domain.ReviewDecision

// Interface aliases.
type AgentSpec = domain.AgentSpec
type ProcessRunner = domain.ProcessRunner
type SessionManager = domain.SessionManager
type AgentRuntime = domain.AgentRuntime

// Agent constants.
const (
	AgentClaude = domain.AgentClaude
	AgentCodex  = domain.AgentCodex
	AgentGemini = domain.AgentGemini
	AgentOllama = domain.AgentOllama
	AgentNone   = domain.AgentNone
)

// TaskStatus constants.
const (
	TaskPending    = domain.TaskPending
	TaskExecuting  = domain.TaskExecuting
	TaskReviewing  = domain.TaskReviewing
	TaskDone       = domain.TaskDone
	TaskFailed     = domain.TaskFailed
	TaskStatusOpen = domain.TaskStatusOpen
	TaskStatusDone = domain.TaskStatusDone
)

// RunnerMode constants.
const (
	RunnerTMUX   = domain.RunnerTMUX
	RunnerPTY    = domain.RunnerPTY
	RunnerDirect = domain.RunnerDirect
)

// IsolationMode constants.
const (
	IsolationWorktree = domain.IsolationWorktree
	IsolationOff      = domain.IsolationOff
)

// ExecutorResult constants.
const (
	ExecutorResultPass    = domain.ExecutorResultPass
	ExecutorResultFail    = domain.ExecutorResultFail
	ExecutorResultUnknown = domain.ExecutorResultUnknown
)

// Validation maps — re-exported for internal loop use.
var validExecutors = domain.ValidExecutors
var validReviewers = domain.ValidReviewers

// normalizeAgent delegates to domain.NormalizeAgent.
func normalizeAgent(agent Agent) Agent {
	return domain.NormalizeAgent(agent)
}
