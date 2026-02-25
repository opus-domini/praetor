package loop

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// LoadPlan reads and validates a plan file.
func LoadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read plan file: %w", err)
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, fmt.Errorf("decode plan file: %w", err)
	}

	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// ValidatePlan validates logical constraints for a plan.
func ValidatePlan(plan Plan) error {
	if len(plan.Tasks) == 0 {
		return errors.New("plan validation failed:\n- tasks array cannot be empty")
	}

	errorsList := make([]string, 0)
	taskIDs := make(map[string]int, len(plan.Tasks))

	for idx, task := range plan.Tasks {
		title := strings.TrimSpace(task.Title)
		if title == "" {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: title is required", idx))
		}

		if task.ID != "" {
			if prev, exists := taskIDs[task.ID]; exists {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: duplicated id %q already used by tasks[%d]", idx, task.ID, prev))
			} else {
				taskIDs[task.ID] = idx
			}
		}

		if task.Executor != "" {
			if _, ok := validExecutors[normalizeAgent(task.Executor)]; !ok {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: invalid executor %q (allowed: codex, claude)", idx, task.Executor))
			}
		}

		if task.Reviewer != "" {
			if _, ok := validReviewers[normalizeAgent(task.Reviewer)]; !ok {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: invalid reviewer %q (allowed: codex, claude, none)", idx, task.Reviewer))
			}
		}
	}

	knownIDs := make(map[string]struct{}, len(plan.Tasks))
	for idx, task := range plan.Tasks {
		id := canonicalTaskID(task, idx)
		knownIDs[id] = struct{}{}
	}

	for idx, task := range plan.Tasks {
		for _, dep := range task.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: depends_on contains an empty id", idx))
				continue
			}
			if _, ok := knownIDs[dep]; !ok {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: depends_on references unknown task id %q", idx, dep))
			}
		}
	}

	if len(errorsList) == 0 {
		return nil
	}

	sort.Strings(errorsList)
	return errors.New("plan validation failed:\n- " + strings.Join(errorsList, "\n- "))
}

func canonicalTaskID(task Task, index int) string {
	id := strings.TrimSpace(task.ID)
	if id != "" {
		return id
	}
	return fmt.Sprintf("auto-%d", index)
}

// PlanChecksum computes a stable checksum for the immutable plan file.
func PlanChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read plan file for checksum: %w", err)
	}
	hash := sha1.Sum(data)
	return hex.EncodeToString(hash[:]), nil
}

// NewPlanFile creates a skeleton plan file in docs/plans.
func NewPlanFile(slug string, now time.Time, baseDir string) (string, error) {
	slug = strings.TrimSpace(slug)
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid slug %q (allowed: lowercase letters, digits, hyphens)", slug)
	}

	plansDir := filepath.Join(baseDir, "docs", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return "", fmt.Errorf("create docs/plans directory: %w", err)
	}

	filename := fmt.Sprintf("PLAN-PRAETOR-%s-%s.json", now.Format("2006-01-02"), slug)
	path := filepath.Join(plansDir, filename)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("plan file already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check plan file: %w", err)
	}

	plan := Plan{
		Schema: "../schemas/loop-plan.schema.json",
		Title:  strings.ReplaceAll(slug, "-", " "),
		Tasks: []Task{
			{
				ID:          "TASK-001",
				Title:       "First task",
				Executor:    AgentCodex,
				Reviewer:    AgentClaude,
				Description: "TODO: describe what this task should do.",
			},
			{
				ID:          "TASK-002",
				Title:       "Second task",
				DependsOn:   []string{"TASK-001"},
				Executor:    AgentCodex,
				Reviewer:    AgentClaude,
				Description: "TODO: describe what this task should do.",
			},
		},
	}

	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode plan skeleton: %w", err)
	}
	encoded = append(encoded, '\n')

	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return "", fmt.Errorf("write plan file: %w", err)
	}
	return path, nil
}

func normalizeAgent(agent Agent) Agent {
	return Agent(strings.ToLower(strings.TrimSpace(string(agent))))
}
