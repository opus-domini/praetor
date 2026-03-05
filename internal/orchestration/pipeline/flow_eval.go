package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

const (
	evalVerdictPass = "pass"
	evalVerdictWarn = "warn"
	evalVerdictFail = "fail"
)

// PlanTaskEvalResult captures local execution quality for one task in one run.
type PlanTaskEvalResult struct {
	TaskID                string   `json:"task_id"`
	Title                 string   `json:"title,omitempty"`
	Status                string   `json:"status"`
	Attempts              int      `json:"attempts"`
	Accepted              bool     `json:"accepted"`
	RequiredGatePass      bool     `json:"required_gate_pass"`
	RequiredGateFailures  int      `json:"required_gate_failures"`
	RequiredGateMissing   int      `json:"required_gate_missing"`
	ParseErrorCount       int      `json:"parse_error_count"`
	StallCount            int      `json:"stall_count"`
	CostUSD               float64  `json:"cost_usd"`
	DurationS             float64  `json:"duration_s"`
	FailureReasons        []string `json:"failure_reasons,omitempty"`
	ObservedRequiredGates []string `json:"observed_required_gates,omitempty"`
}

// PlanEvalSummary provides a local quality verdict for one plan run.
type PlanEvalSummary struct {
	TaskCount                int      `json:"task_count"`
	DoneCount                int      `json:"done_count"`
	FailedCount              int      `json:"failed_count"`
	AcceptedCount            int      `json:"accepted_count"`
	AcceptanceRate           float64  `json:"acceptance_rate"`
	RequiredGateFailureTasks int      `json:"required_gate_failure_tasks"`
	RequiredGateMissingTasks int      `json:"required_gate_missing_tasks"`
	ParseErrorTasks          int      `json:"parse_error_tasks"`
	StalledTasks             int      `json:"stalled_tasks"`
	AvgRetriesPerTask        float64  `json:"avg_retries_per_task"`
	AvgCostUSDPerTask        float64  `json:"avg_cost_usd_per_task"`
	TotalCostUSD             float64  `json:"total_cost_usd"`
	P95TaskDurationS         float64  `json:"p95_task_duration_s"`
	RunDurationS             float64  `json:"run_duration_s"`
	Verdict                  string   `json:"verdict"`
	Reasons                  []string `json:"reasons,omitempty"`
}

// PlanEvalReport is the full local evaluation output for one plan run.
type PlanEvalReport struct {
	PlanSlug         string               `json:"plan_slug"`
	RunID            string               `json:"run_id"`
	RunDir           string               `json:"run_dir"`
	ProjectHome      string               `json:"project_home"`
	PlanName         string               `json:"plan_name,omitempty"`
	Outcome          string               `json:"outcome,omitempty"`
	Timestamp        string               `json:"timestamp,omitempty"`
	RequiredGates    []string             `json:"required_gates,omitempty"`
	Tasks            []PlanTaskEvalResult `json:"tasks"`
	Summary          PlanEvalSummary      `json:"summary"`
	MissingArtifacts []string             `json:"missing_artifacts,omitempty"`
}

// ProjectPlanEvalResult captures one plan-level verdict in project aggregation.
type ProjectPlanEvalResult struct {
	PlanSlug       string          `json:"plan_slug"`
	RunID          string          `json:"run_id"`
	Outcome        string          `json:"outcome,omitempty"`
	Timestamp      string          `json:"timestamp,omitempty"`
	Verdict        string          `json:"verdict"`
	AcceptanceRate float64         `json:"acceptance_rate"`
	Summary        PlanEvalSummary `json:"summary"`
}

// ProjectEvalSummary provides local project-level quality aggregation.
type ProjectEvalSummary struct {
	PlanCount         int      `json:"plan_count"`
	PassCount         int      `json:"pass_count"`
	WarnCount         int      `json:"warn_count"`
	FailCount         int      `json:"fail_count"`
	TaskCount         int      `json:"task_count"`
	AcceptedTaskCount int      `json:"accepted_task_count"`
	AcceptanceRate    float64  `json:"acceptance_rate"`
	AvgRetriesPerTask float64  `json:"avg_retries_per_task"`
	TotalCostUSD      float64  `json:"total_cost_usd"`
	AvgCostUSDPerPlan float64  `json:"avg_cost_usd_per_plan"`
	Verdict           string   `json:"verdict"`
	Reasons           []string `json:"reasons,omitempty"`
}

// ProjectEvalReport aggregates latest local run quality per plan.
type ProjectEvalReport struct {
	ProjectHome string                  `json:"project_home"`
	GeneratedAt string                  `json:"generated_at"`
	Window      string                  `json:"window,omitempty"`
	Plans       []ProjectPlanEvalResult `json:"plans"`
	Summary     ProjectEvalSummary      `json:"summary"`
}

type flowEvalTaskAccumulator struct {
	firstTS            time.Time
	lastTS             time.Time
	costUSD            float64
	stalls             int
	parseErrors        int
	requiredGateStatus map[string]string
}

// EvaluatePlanFlow analyzes one local plan run end-to-end (tasks, gates, parse errors, retries, and cost).
func EvaluatePlanFlow(store *localstate.Store, slug, runID string) (PlanEvalReport, error) {
	if store == nil {
		return PlanEvalReport{}, errors.New("store is required")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return PlanEvalReport{}, errors.New("plan slug is required")
	}

	plan, err := domain.LoadPlan(store.PlanFile(slug))
	if err != nil {
		return PlanEvalReport{}, err
	}
	requiredGates := normalizeGateList(plan.Quality.Required)

	report := PlanEvalReport{
		PlanSlug:      slug,
		ProjectHome:   strings.TrimSpace(store.Root),
		PlanName:      strings.TrimSpace(plan.Name),
		RequiredGates: requiredGates,
		Tasks:         make([]PlanTaskEvalResult, 0),
	}

	runtimeRoot := store.RuntimeDir()
	var snapshot localstate.LocalSnapshot
	var runDir string

	runID = strings.TrimSpace(runID)
	if runID == "" {
		latest, snapshotPath, loadErr := localstate.LoadLatestLocalSnapshot(runtimeRoot, slug)
		if loadErr != nil {
			return PlanEvalReport{}, loadErr
		}
		if strings.TrimSpace(snapshotPath) == "" {
			return PlanEvalReport{}, fmt.Errorf("no runtime snapshots found for plan: %s", slug)
		}
		snapshot = latest
		runDir = filepath.Dir(snapshotPath)
		runID = filepath.Base(runDir)
	} else {
		runDir = filepath.Join(runtimeRoot, runID)
		snap, loadErr := loadLocalSnapshot(runDir)
		if loadErr != nil {
			return PlanEvalReport{}, loadErr
		}
		snapshot = snap
		if strings.TrimSpace(snapshot.PlanSlug) != slug {
			return PlanEvalReport{}, fmt.Errorf("run %s belongs to plan %s, expected %s", runID, snapshot.PlanSlug, slug)
		}
	}

	report.RunID = runID
	report.RunDir = runDir
	report.Outcome = strings.TrimSpace(string(snapshot.Outcome))
	report.Timestamp = strings.TrimSpace(snapshot.Timestamp)

	eventsPath := filepath.Join(runDir, "events.jsonl")
	events, err := readFlowJSONL(eventsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return PlanEvalReport{}, fmt.Errorf("read events.jsonl: %w", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		report.MissingArtifacts = append(report.MissingArtifacts, "events.jsonl")
	}

	taskOrder := make([]string, 0, len(snapshot.State.Tasks))
	taskResult := make(map[string]PlanTaskEvalResult, len(snapshot.State.Tasks))
	taskAccum := make(map[string]*flowEvalTaskAccumulator, len(snapshot.State.Tasks))
	for _, task := range snapshot.State.Tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		taskOrder = append(taskOrder, id)
		taskResult[id] = PlanTaskEvalResult{
			TaskID:           id,
			Title:            strings.TrimSpace(task.Title),
			Status:           strings.TrimSpace(string(task.Status)),
			Attempts:         task.Attempt,
			RequiredGatePass: true,
		}
		taskAccum[id] = &flowEvalTaskAccumulator{
			requiredGateStatus: make(map[string]string),
		}
	}

	runFirstTS := time.Time{}
	runLastTS := time.Time{}
	for _, event := range events {
		eventTS := parseFlowEventTimestamp(event)
		if !eventTS.IsZero() {
			if runFirstTS.IsZero() || eventTS.Before(runFirstTS) {
				runFirstTS = eventTS
			}
			if runLastTS.IsZero() || eventTS.After(runLastTS) {
				runLastTS = eventTS
			}
		}

		taskID := strings.TrimSpace(flowString(event["task_id"]))
		if taskID == "" {
			if data := flowAnyMap(event["data"]); len(data) > 0 {
				taskID = strings.TrimSpace(flowString(data["task_id"]))
			}
		}
		if taskID == "" {
			continue
		}
		current, ok := taskResult[taskID]
		if !ok {
			continue
		}
		accum := taskAccum[taskID]
		if accum == nil {
			accum = &flowEvalTaskAccumulator{requiredGateStatus: make(map[string]string)}
			taskAccum[taskID] = accum
		}

		if !eventTS.IsZero() {
			if accum.firstTS.IsZero() || eventTS.Before(accum.firstTS) {
				accum.firstTS = eventTS
			}
			if accum.lastTS.IsZero() || eventTS.After(accum.lastTS) {
				accum.lastTS = eventTS
			}
		}
		accum.costUSD += flowFloat(event["cost_usd"])

		eventType := strings.ToLower(strings.TrimSpace(flowEventType(event)))
		switch eventType {
		case "task_stalled":
			accum.stalls++
		case "agent_error":
			if data := flowAnyMap(event["data"]); len(data) > 0 {
				if strings.TrimSpace(flowString(data["parse_error_class"])) != "" {
					accum.parseErrors++
				}
			}
		case "gate_result":
			data := flowAnyMap(event["data"])
			if len(data) == 0 {
				break
			}
			if !flowBool(data["required"]) {
				break
			}
			gateName := strings.ToLower(strings.TrimSpace(flowString(data["gate"])))
			if gateName == "" {
				break
			}
			status := strings.ToUpper(strings.TrimSpace(flowString(data["status"])))
			if status == "" {
				status = strings.ToUpper(strings.TrimSpace(flowString(event["action"])))
			}
			if status == "" {
				status = "UNKNOWN"
			}
			currentStatus, exists := accum.requiredGateStatus[gateName]
			if !exists || flowGateWorse(status, currentStatus) {
				accum.requiredGateStatus[gateName] = status
			}
		}

		current.StallCount = accum.stalls
		current.ParseErrorCount = accum.parseErrors
		current.CostUSD = accum.costUSD
		taskResult[taskID] = current
	}

	durations := make([]float64, 0, len(taskOrder))
	acceptedCount := 0
	requiredFailTasks := 0
	requiredMissingTasks := 0
	parseErrorTasks := 0
	stalledTasks := 0
	totalRetries := 0
	totalCost := 0.0

	for _, taskID := range taskOrder {
		result := taskResult[taskID]
		accum := taskAccum[taskID]
		if accum == nil {
			accum = &flowEvalTaskAccumulator{requiredGateStatus: make(map[string]string)}
			taskAccum[taskID] = accum
		}

		if !accum.firstTS.IsZero() && !accum.lastTS.IsZero() && !accum.lastTS.Before(accum.firstTS) {
			result.DurationS = accum.lastTS.Sub(accum.firstTS).Seconds()
		}
		durations = append(durations, result.DurationS)
		result.StallCount = accum.stalls
		result.ParseErrorCount = accum.parseErrors
		result.CostUSD = accum.costUSD

		if len(requiredGates) > 0 {
			result.ObservedRequiredGates = make([]string, 0, len(accum.requiredGateStatus))
			for gateName, status := range accum.requiredGateStatus {
				result.ObservedRequiredGates = append(result.ObservedRequiredGates, fmt.Sprintf("%s:%s", gateName, status))
			}
			sort.Strings(result.ObservedRequiredGates)
			for _, gate := range requiredGates {
				status, exists := accum.requiredGateStatus[gate]
				if !exists {
					result.RequiredGatePass = false
					result.RequiredGateMissing++
					result.FailureReasons = append(result.FailureReasons, "missing required gate: "+gate)
					continue
				}
				if strings.ToUpper(strings.TrimSpace(status)) != "PASS" {
					result.RequiredGatePass = false
					result.RequiredGateFailures++
					result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("required gate %s status=%s", gate, status))
				}
			}
		}

		if result.ParseErrorCount > 0 {
			result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("%d parse error event(s)", result.ParseErrorCount))
		}

		status := strings.ToLower(strings.TrimSpace(result.Status))
		result.Accepted = status == string(domain.TaskDone) &&
			result.RequiredGateFailures == 0 &&
			result.RequiredGateMissing == 0 &&
			result.ParseErrorCount == 0
		if !result.Accepted && len(result.FailureReasons) == 0 && status != string(domain.TaskDone) {
			result.FailureReasons = append(result.FailureReasons, "task not in done status")
		}

		if result.Accepted {
			acceptedCount++
		}
		if result.RequiredGateFailures > 0 {
			requiredFailTasks++
		}
		if result.RequiredGateMissing > 0 {
			requiredMissingTasks++
		}
		if result.ParseErrorCount > 0 {
			parseErrorTasks++
		}
		if result.StallCount > 0 {
			stalledTasks++
		}

		totalRetries += result.Attempts
		totalCost += result.CostUSD
		report.Tasks = append(report.Tasks, result)
	}

	summary := PlanEvalSummary{
		TaskCount:                len(report.Tasks),
		DoneCount:                snapshot.State.DoneCount(),
		FailedCount:              snapshot.State.FailedCount(),
		AcceptedCount:            acceptedCount,
		RequiredGateFailureTasks: requiredFailTasks,
		RequiredGateMissingTasks: requiredMissingTasks,
		ParseErrorTasks:          parseErrorTasks,
		StalledTasks:             stalledTasks,
		TotalCostUSD:             totalCost,
		P95TaskDurationS:         flowP95(durations),
		Verdict:                  evalVerdictPass,
	}
	if summary.TaskCount > 0 {
		summary.AcceptanceRate = float64(summary.AcceptedCount) / float64(summary.TaskCount)
		summary.AvgRetriesPerTask = float64(totalRetries) / float64(summary.TaskCount)
		summary.AvgCostUSDPerTask = summary.TotalCostUSD / float64(summary.TaskCount)
	}
	if !runFirstTS.IsZero() && !runLastTS.IsZero() && !runLastTS.Before(runFirstTS) {
		summary.RunDurationS = runLastTS.Sub(runFirstTS).Seconds()
	}
	summary = flowApplyPlanVerdictRules(summary)
	report.Summary = summary
	return report, nil
}

// EvaluateProjectFlow aggregates latest local run quality for each plan.
func EvaluateProjectFlow(store *localstate.Store, window time.Duration) (ProjectEvalReport, error) {
	if store == nil {
		return ProjectEvalReport{}, errors.New("store is required")
	}
	slugs, err := store.ListPlanSlugs()
	if err != nil {
		return ProjectEvalReport{}, err
	}

	report := ProjectEvalReport{
		ProjectHome: strings.TrimSpace(store.Root),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Plans:       make([]ProjectPlanEvalResult, 0, len(slugs)),
		Summary: ProjectEvalSummary{
			Verdict: evalVerdictPass,
		},
	}
	if window > 0 {
		report.Window = window.String()
	}

	cutoff := time.Time{}
	if window > 0 {
		cutoff = time.Now().Add(-window)
	}

	totalRetries := 0.0
	totalTasks := 0.0
	totalAccepted := 0.0
	totalCost := 0.0
	failReasons := make([]string, 0)
	warnReasons := make([]string, 0)

	for _, slug := range slugs {
		snapshot, path, loadErr := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), slug)
		if loadErr != nil {
			return ProjectEvalReport{}, loadErr
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		timestamp := localstate.ParseTimestamp(snapshot.Timestamp)
		if !cutoff.IsZero() && !timestamp.IsZero() && timestamp.Before(cutoff) {
			continue
		}

		runID := filepath.Base(filepath.Dir(path))
		planReport, planErr := EvaluatePlanFlow(store, slug, runID)
		if planErr != nil {
			return ProjectEvalReport{}, planErr
		}

		report.Plans = append(report.Plans, ProjectPlanEvalResult{
			PlanSlug:       slug,
			RunID:          runID,
			Outcome:        planReport.Outcome,
			Timestamp:      planReport.Timestamp,
			Verdict:        planReport.Summary.Verdict,
			AcceptanceRate: planReport.Summary.AcceptanceRate,
			Summary:        planReport.Summary,
		})

		report.Summary.PlanCount++
		totalTasks += float64(planReport.Summary.TaskCount)
		totalAccepted += float64(planReport.Summary.AcceptedCount)
		totalRetries += planReport.Summary.AvgRetriesPerTask * float64(planReport.Summary.TaskCount)
		totalCost += planReport.Summary.TotalCostUSD

		switch strings.ToLower(strings.TrimSpace(planReport.Summary.Verdict)) {
		case evalVerdictFail:
			report.Summary.FailCount++
			failReasons = append(failReasons, fmt.Sprintf("%s failed quality checks", slug))
		case evalVerdictWarn:
			report.Summary.WarnCount++
			warnReasons = append(warnReasons, fmt.Sprintf("%s has reliability warnings", slug))
		default:
			report.Summary.PassCount++
		}
	}

	report.Summary.TaskCount = int(totalTasks)
	report.Summary.AcceptedTaskCount = int(totalAccepted)
	if totalTasks > 0 {
		report.Summary.AcceptanceRate = totalAccepted / totalTasks
		report.Summary.AvgRetriesPerTask = totalRetries / totalTasks
	}
	report.Summary.TotalCostUSD = totalCost
	if report.Summary.PlanCount > 0 {
		report.Summary.AvgCostUSDPerPlan = totalCost / float64(report.Summary.PlanCount)
	}

	switch {
	case report.Summary.FailCount > 0:
		report.Summary.Verdict = evalVerdictFail
		report.Summary.Reasons = failReasons
	case report.Summary.WarnCount > 0:
		report.Summary.Verdict = evalVerdictWarn
		report.Summary.Reasons = warnReasons
	case report.Summary.PlanCount == 0:
		report.Summary.Verdict = evalVerdictWarn
		report.Summary.Reasons = []string{"no local plan runs found in selected window"}
	default:
		report.Summary.Verdict = evalVerdictPass
	}

	sort.Slice(report.Plans, func(i, j int) bool {
		if report.Plans[i].Verdict == report.Plans[j].Verdict {
			return report.Plans[i].PlanSlug < report.Plans[j].PlanSlug
		}
		return flowVerdictRank(report.Plans[i].Verdict) > flowVerdictRank(report.Plans[j].Verdict)
	})

	return report, nil
}

func flowApplyPlanVerdictRules(summary PlanEvalSummary) PlanEvalSummary {
	reasons := make([]string, 0)
	verdict := evalVerdictPass

	if summary.FailedCount > 0 {
		verdict = evalVerdictFail
		reasons = append(reasons, fmt.Sprintf("%d task(s) failed", summary.FailedCount))
	}
	if summary.RequiredGateFailureTasks > 0 {
		verdict = evalVerdictFail
		reasons = append(reasons, fmt.Sprintf("%d task(s) failed required gates", summary.RequiredGateFailureTasks))
	}
	if summary.RequiredGateMissingTasks > 0 {
		verdict = evalVerdictFail
		reasons = append(reasons, fmt.Sprintf("%d task(s) missing required gate evidence", summary.RequiredGateMissingTasks))
	}
	if summary.ParseErrorTasks > 0 {
		verdict = evalVerdictFail
		reasons = append(reasons, fmt.Sprintf("%d task(s) had parse error events", summary.ParseErrorTasks))
	}

	if verdict != evalVerdictFail {
		if summary.StalledTasks > 0 {
			verdict = evalVerdictWarn
			reasons = append(reasons, fmt.Sprintf("%d task(s) stalled", summary.StalledTasks))
		}
		if summary.AvgRetriesPerTask > 0 {
			verdict = evalVerdictWarn
			reasons = append(reasons, fmt.Sprintf("avg retries per task is %.2f", summary.AvgRetriesPerTask))
		}
	}

	summary.Verdict = verdict
	summary.Reasons = reasons
	return summary
}

func loadLocalSnapshot(runDir string) (localstate.LocalSnapshot, error) {
	path := filepath.Join(strings.TrimSpace(runDir), "snapshot.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return localstate.LocalSnapshot{}, fmt.Errorf("read snapshot.json: %w", err)
	}

	snapshot := localstate.Snapshot{}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return localstate.LocalSnapshot{}, fmt.Errorf("decode snapshot.json: %w", err)
	}

	state := domain.State{}
	if len(snapshot.State) > 0 {
		if err := json.Unmarshal(snapshot.State, &state); err != nil {
			return localstate.LocalSnapshot{}, fmt.Errorf("decode snapshot state: %w", err)
		}
	}

	return localstate.LocalSnapshot{
		Version:           snapshot.Version,
		RunID:             snapshot.RunID,
		PlanSlug:          snapshot.PlanSlug,
		PlanChecksum:      snapshot.PlanChecksum,
		ProjectRoot:       snapshot.ProjectRoot,
		ManifestPath:      snapshot.ManifestPath,
		ManifestHash:      snapshot.ManifestHash,
		ManifestTruncated: snapshot.ManifestTruncated,
		Phase:             snapshot.Phase,
		Message:           snapshot.Message,
		Outcome:           domain.RunOutcome(snapshot.Outcome),
		Iteration:         snapshot.Iteration,
		Timestamp:         snapshot.Timestamp,
		State:             state,
	}, nil
}

func normalizeGateList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, gate := range values {
		name := strings.ToLower(strings.TrimSpace(gate))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func readFlowJSONL(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		item := map[string]any{}
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue
		}
		records = append(records, item)
	}
	return records, nil
}

func parseFlowEventTimestamp(event map[string]any) time.Time {
	raw := strings.TrimSpace(flowString(event["timestamp"]))
	if raw == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func flowEventType(event map[string]any) string {
	if value := strings.TrimSpace(flowString(event["event_type"])); value != "" {
		return value
	}
	return strings.TrimSpace(flowString(event["type"]))
}

func flowAnyMap(value any) map[string]any {
	switch v := value.(type) {
	case map[string]any:
		return v
	default:
		return nil
	}
}

func flowString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return ""
	}
}

func flowFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func flowBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func flowGateWorse(candidate, existing string) bool {
	return flowGateRank(candidate) > flowGateRank(existing)
}

func flowGateRank(status string) int {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PASS":
		return 0
	case "MISSING":
		return 1
	case "WARN":
		return 2
	case "FAIL":
		return 3
	case "ERROR":
		return 4
	default:
		return 5
	}
}

func flowP95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	normalized := make([]float64, len(values))
	copy(normalized, values)
	sort.Float64s(normalized)

	index := int(math.Ceil(0.95*float64(len(normalized)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(normalized) {
		index = len(normalized) - 1
	}
	return normalized[index]
}

func flowVerdictRank(verdict string) int {
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case evalVerdictFail:
		return 3
	case evalVerdictWarn:
		return 2
	case evalVerdictPass:
		return 1
	default:
		return 0
	}
}
