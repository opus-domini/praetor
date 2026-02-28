package prompt

// ExecutorSystemData holds template data for the executor system prompt.
type ExecutorSystemData struct {
	ProjectContext string
}

// ExecutorTaskData holds template data for the executor task prompt.
type ExecutorTaskData struct {
	IsRetry          bool
	RetryAttempt     int
	PreviousFeedback string
	TaskTitle        string
	TaskID           string
	TaskIndex        int
	TaskDependsOn    string
	TaskDescription  string
	TaskAcceptance   string
	PlanFile         string
	PlanName         string
	PlanProgress     string
	Workdir          string
	GatesRequired    []string
	EvidenceFormat   string
}

// ReviewerSystemData holds template data for the reviewer system prompt.
type ReviewerSystemData struct {
	ProjectContext string
}

// ReviewerTaskData holds template data for the reviewer task prompt.
type ReviewerTaskData struct {
	TaskTitle       string
	TaskID          string
	TaskDependsOn   string
	TaskDescription string
	TaskAcceptance  string
	PlanFile        string
	PlanName        string
	PlanProgress    string
	Workdir         string
	ExecutorOutput  string
	GitDiff         string
}

// PlannerSystemData holds template data for the planner system prompt.
type PlannerSystemData struct {
	ProjectContext string
}

// PlannerTaskData holds template data for the planner task prompt.
type PlannerTaskData struct {
	Objective string
}

// AdapterPlanData holds template data for adapter Plan() prompts.
type AdapterPlanData struct {
	Objective        string
	WorkspaceContext string
}
