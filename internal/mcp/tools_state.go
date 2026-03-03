package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opus-domini/praetor/internal/state"
)

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

			// Find the latest run directory.
			snapshot, _, err := state.LoadLatestSnapshot(store.RuntimeDir(), slug)
			if err != nil {
				return nil, fmt.Errorf("no runs found for plan %q: %w", slug, err)
			}
			if snapshot.RunID == "" {
				return nil, fmt.Errorf("no runs found for plan %q", slug)
			}
			eventsPath := filepath.Join(store.RuntimeDir(), snapshot.RunID, "events.log")

			events, err := readLastNJSONL(eventsPath, lastN)
			if err != nil {
				return nil, fmt.Errorf("read events: %w", err)
			}
			return jsonContent(events)
		},
	)

	s.tools.register("plan_diagnose", "Get diagnostics for a plan run (errors, stalls, costs)",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"query":       stringProp("Diagnostic query: errors, stalls, fallbacks, costs, all (default: all)"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			slug := argString(args, "slug")
			query := argString(args, "query")
			if query == "" {
				query = "all"
			}

			snapshot, _, err := state.LoadLatestSnapshot(store.RuntimeDir(), slug)
			if err != nil {
				return nil, fmt.Errorf("no runs found for plan %q: %w", slug, err)
			}
			if snapshot.RunID == "" {
				return nil, fmt.Errorf("no runs found for plan %q", slug)
			}
			eventsPath := filepath.Join(store.RuntimeDir(), snapshot.RunID, "events.log")

			events, err := readAllJSONL(eventsPath)
			if err != nil {
				return nil, fmt.Errorf("read events: %w", err)
			}

			filtered := filterEventsByQuery(events, query)
			return jsonContent(filtered)
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

	var result []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		result = append(result, entry)
	}
	return result, scanner.Err()
}

func filterEventsByQuery(events []map[string]any, query string) []map[string]any {
	if query == "all" {
		return events
	}
	var result []map[string]any
	for _, event := range events {
		eventType, _ := event["event_type"].(string)
		switch query {
		case "errors":
			if eventType == "agent_error" {
				result = append(result, event)
			}
		case "stalls":
			if eventType == "task_stalled" {
				result = append(result, event)
			}
		case "fallbacks":
			if eventType == "agent_fallback" {
				result = append(result, event)
			}
		case "costs":
			if eventType == "agent_complete" {
				result = append(result, event)
			}
		}
	}
	return result
}
