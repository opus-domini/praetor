package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/state"
)

type planDiagnoseResponse struct {
	RunID         string              `json:"run_id,omitempty"`
	Query         string              `json:"query"`
	Summary       planDiagnoseSummary `json:"summary"`
	RunSummary    domain.RunSummary   `json:"run_summary,omitempty"`
	ActorAnalysis planActorAnalysis   `json:"actor_analysis,omitempty"`
	Events        []map[string]any    `json:"events,omitempty"`
	Performance   []map[string]any    `json:"performance,omitempty"`
}

type planDiagnoseSummary struct {
	PlanSlug          string  `json:"plan_slug"`
	Outcome           string  `json:"outcome,omitempty"`
	Done              int     `json:"done"`
	Failed            int     `json:"failed"`
	Active            int     `json:"active"`
	Total             int     `json:"total"`
	Running           bool    `json:"running"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	PlanLimitUSD      float64 `json:"plan_limit_usd,omitempty"`
	TaskLimitUSD      float64 `json:"task_limit_usd,omitempty"`
	CostBudgetEnforce bool    `json:"cost_budget_enforce"`
}

type planActorAnalysis struct {
	MostRetriesActor string             `json:"most_retries_actor,omitempty"`
	MostRetriesCount int                `json:"most_retries_count,omitempty"`
	MostStallsActor  string             `json:"most_stalls_actor,omitempty"`
	MostStallsCount  int                `json:"most_stalls_count,omitempty"`
	CostByActor      map[string]float64 `json:"cost_by_actor,omitempty"`
}

func registerStateTools(s *Server) {
	s.tools.register("plan_events", "Get execution events from a plan run",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"last_n":      intProp("Number of recent events to return (default: 50)"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			slug := argString(args, "slug")
			lastN := argInt(args, "last_n", 50)

			snapshot, _, err := state.LoadLatestLocalSnapshot(store.RuntimeDir(), slug)
			if err != nil {
				return nil, fmt.Errorf("no runs found for plan %q: %w", slug, err)
			}
			if strings.TrimSpace(snapshot.RunID) == "" {
				return nil, fmt.Errorf("no runs found for plan %q", slug)
			}
			eventsPath := filepath.Join(store.RuntimeDir(), snapshot.RunID, "events.jsonl")

			events, err := readLastNJSONL(eventsPath, lastN)
			if err != nil {
				return nil, fmt.Errorf("read events: %w", err)
			}
			return jsonContent(events)
		},
	)

	s.tools.register("plan_diagnose", "Get diagnostics for a plan run (errors, stalls, fallbacks, costs, summary)",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"query":       stringProp("Diagnostic query: errors, stalls, fallbacks, costs, summary, all (default: all)"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			slug := argString(args, "slug")
			query := strings.ToLower(strings.TrimSpace(argString(args, "query")))
			if query == "" {
				query = "all"
			}

			status, err := store.Status(slug)
			if err != nil {
				return nil, err
			}

			snapshot, snapshotPath, err := state.LoadLatestLocalSnapshot(store.RuntimeDir(), slug)
			if err != nil {
				return nil, fmt.Errorf("load latest snapshot: %w", err)
			}

			var events []map[string]any
			var perf []map[string]any
			if snapshotPath != "" {
				runDir := filepath.Dir(snapshotPath)
				events, err = readAllJSONL(filepath.Join(runDir, "events.jsonl"))
				if err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("read events: %w", err)
				}
				perf, err = readAllJSONL(filepath.Join(runDir, "diagnostics", "performance.jsonl"))
				if err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("read performance diagnostics: %w", err)
				}
			}

			response := planDiagnoseResponse{
				RunID:         snapshot.RunID,
				Query:         query,
				Summary:       buildPlanDiagnoseSummary(status),
				RunSummary:    snapshot.Summary,
				ActorAnalysis: buildPlanActorAnalysis(snapshot.Summary, events),
			}
			if query != "summary" {
				response.Events = filterEventsByQuery(events, query)
			}
			if query == "all" || query == "costs" || query == "summary" {
				response.Performance = perf
			}
			return jsonContent(response)
		},
	)
}

func readLastNJSONL(path string, n int) ([]map[string]any, error) {
	all, err := readAllJSONL(path)
	if err != nil {
		return nil, err
	}
	if n > 0 && len(all) > n {
		return all[len(all)-n:], nil
	}
	return all, nil
}

func readAllJSONL(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	records := make([]map[string]any, 0)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		record := map[string]any{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func filterEventsByQuery(events []map[string]any, query string) []map[string]any {
	if query == "all" || query == "summary" {
		return events
	}
	filtered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		eventType := strings.ToLower(strings.TrimSpace(stringValue(event["event_type"])))
		if eventType == "" {
			eventType = strings.ToLower(strings.TrimSpace(stringValue(event["type"])))
		}
		switch query {
		case "errors":
			if strings.Contains(eventType, "error") || eventType == "task_failed" {
				filtered = append(filtered, event)
			}
		case "stalls":
			if eventType == "task_stalled" {
				filtered = append(filtered, event)
			}
		case "fallbacks":
			if eventType == "agent_fallback" {
				filtered = append(filtered, event)
			}
		case "costs":
			if anyFloat(event["cost_usd"]) > 0 || eventType == "cost_budget_warning" || eventType == "cost_budget_exceeded" {
				filtered = append(filtered, event)
			}
		}
	}
	return filtered
}

func buildPlanDiagnoseSummary(status domain.PlanStatus) planDiagnoseSummary {
	return planDiagnoseSummary{
		PlanSlug:          status.PlanSlug,
		Outcome:           string(status.Outcome),
		Done:              status.Done,
		Failed:            status.Failed,
		Active:            status.Active,
		Total:             status.Total,
		Running:           status.Running,
		TotalCostUSD:      microsToUSD(status.TotalCostMicros),
		PlanLimitUSD:      centsToUSD(status.ExecutionPolicy.Cost.PlanLimitCents),
		TaskLimitUSD:      centsToUSD(status.ExecutionPolicy.Cost.TaskLimitCents),
		CostBudgetEnforce: costBudgetEnforced(status.ExecutionPolicy.Cost),
	}
}

func buildPlanActorAnalysis(summary domain.RunSummary, events []map[string]any) planActorAnalysis {
	analysis := planActorAnalysis{
		CostByActor: make(map[string]float64),
	}

	for actor, stats := range summary.ByActor {
		if stats.CostUSD > 0 {
			analysis.CostByActor[actor] = stats.CostUSD
		}
		if stats.Retries > analysis.MostRetriesCount || (stats.Retries == analysis.MostRetriesCount && actor < analysis.MostRetriesActor) {
			analysis.MostRetriesActor = actor
			analysis.MostRetriesCount = stats.Retries
		}
		if stats.Stalls > analysis.MostStallsCount || (stats.Stalls == analysis.MostStallsCount && actor < analysis.MostStallsActor) {
			analysis.MostStallsActor = actor
			analysis.MostStallsCount = stats.Stalls
		}
	}

	if len(analysis.CostByActor) == 0 {
		for _, event := range events {
			actor := eventActorKey(event)
			cost := anyFloat(event["cost_usd"])
			if actor != "" && cost > 0 {
				analysis.CostByActor[actor] += cost
			}
		}
	}

	if len(analysis.CostByActor) == 0 {
		analysis.CostByActor = nil
	}
	return analysis
}

func eventActorKey(event map[string]any) string {
	role := strings.TrimSpace(stringValue(event["role"]))
	agent := strings.TrimSpace(stringValue(event["agent"]))
	if actor, ok := event["actor"].(map[string]any); ok {
		if role == "" {
			role = strings.TrimSpace(stringValue(actor["role"]))
		}
		if agent == "" {
			agent = strings.TrimSpace(stringValue(actor["agent"]))
		}
	}
	if role == "" && agent == "" {
		return ""
	}
	if role == "" {
		role = "unknown"
	}
	if agent == "" {
		agent = "unknown"
	}
	return role + ":" + agent
}

func stringValue(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return fmt.Sprint(v)
	}
}

func anyFloat(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	default:
		return 0
	}
}

func microsToUSD(micros int64) float64 {
	if micros <= 0 {
		return 0
	}
	return float64(micros) / 1_000_000
}

func centsToUSD(cents int64) float64 {
	if cents <= 0 {
		return 0
	}
	return float64(cents) / 100
}

func costBudgetEnforced(policy domain.CostPolicy) bool {
	return policy.Enforce == nil || *policy.Enforce
}
