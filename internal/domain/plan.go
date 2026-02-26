package domain

import (
	"crypto/sha256"
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

// CanonicalTaskID returns the stable identifier for a plan task.
// If the task has an explicit ID it is used; otherwise an auto-generated hash ID is derived.
func CanonicalTaskID(task Task, index int) string {
	id := strings.TrimSpace(task.ID)
	if id != "" {
		return id
	}
	return autoTaskID(task)
}

func autoTaskID(task Task) string {
	payload := AutoTaskFingerprint(
		task.Title,
		task.Executor,
		task.Reviewer,
		task.Model,
		task.Description,
		task.Criteria,
		task.DependsOn,
	)
	hash := sha256.Sum256([]byte(payload))
	return "auto-" + hex.EncodeToString(hash[:])[:12]
}

// AutoTaskFingerprint builds the stable fingerprint string used to match
// implicit-ID tasks across plan edits.
func AutoTaskFingerprint(title string, executor Agent, reviewer Agent, model, description, criteria string, dependsOn []string) string {
	normalizedDeps := make([]string, 0, len(dependsOn))
	for _, dep := range dependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		normalizedDeps = append(normalizedDeps, dep)
	}
	sort.Strings(normalizedDeps)

	parts := []string{
		strings.TrimSpace(title),
		string(NormalizeAgent(executor)),
		string(NormalizeAgent(reviewer)),
		strings.TrimSpace(model),
		strings.TrimSpace(description),
		strings.TrimSpace(criteria),
		strings.Join(normalizedDeps, ","),
	}
	return strings.Join(parts, "\n")
}

// StateTasksFromPlan creates state tasks from plan tasks using canonical IDs
// and normalized agent values.
func StateTasksFromPlan(plan Plan) []StateTask {
	tasks := make([]StateTask, 0, len(plan.Tasks))
	for i, task := range plan.Tasks {
		tasks = append(tasks, StateTask{
			ID:          CanonicalTaskID(task, i),
			Title:       strings.TrimSpace(task.Title),
			DependsOn:   NormalizedDependsOn(task.DependsOn),
			Executor:    NormalizeAgent(task.Executor),
			Reviewer:    NormalizeAgent(task.Reviewer),
			Model:       strings.TrimSpace(task.Model),
			Description: strings.TrimSpace(task.Description),
			Criteria:    strings.TrimSpace(task.Criteria),
			Status:      TaskPending,
		})
	}
	return tasks
}

// NormalizedDependsOn trims and filters empty dependency IDs.
func NormalizedDependsOn(dependsOn []string) []string {
	if len(dependsOn) == 0 {
		return nil
	}
	result := make([]string, 0, len(dependsOn))
	for _, dep := range dependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		result = append(result, dep)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// WriteJSONFile atomically writes a value as indented JSON using tmp+rename.
func WriteJSONFile(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json file %s: %w", path, err)
	}
	encoded = append(encoded, '\n')

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, encoded, 0o644); err != nil {
		return fmt.Errorf("write tmp file %s: %w", tmpFile, err)
	}
	if err := os.Rename(tmpFile, path); err != nil {
		return fmt.Errorf("rename tmp file %s: %w", path, err)
	}
	return nil
}

// SanitizePathToken replaces path-unsafe characters with hyphens.
func SanitizePathToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "task"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-")
	return replacer.Replace(value)
}

// PlanChecksum computes a stable SHA-256 checksum for a plan file.
func PlanChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read plan file for checksum: %w", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

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
	canonicalIDs := make(map[string]int, len(plan.Tasks))

	for idx, task := range plan.Tasks {
		title := strings.TrimSpace(task.Title)
		if title == "" {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: title is required", idx))
		}

		id := CanonicalTaskID(task, idx)
		if prev, exists := canonicalIDs[id]; exists {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: duplicated id %q already used by tasks[%d]", idx, id, prev))
		} else {
			canonicalIDs[id] = idx
		}

		if task.Executor != "" {
			if _, ok := ValidExecutors[NormalizeAgent(task.Executor)]; !ok {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: invalid executor %q (allowed: codex, claude, gemini, ollama)", idx, task.Executor))
			}
		}

		if task.Reviewer != "" {
			if _, ok := ValidReviewers[NormalizeAgent(task.Reviewer)]; !ok {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: invalid reviewer %q (allowed: codex, claude, gemini, ollama, none)", idx, task.Reviewer))
			}
		}

		if task.Model != "" && strings.TrimSpace(task.Model) == "" {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: model cannot be blank", idx))
		}
	}

	knownIDs := make(map[string]struct{}, len(plan.Tasks))
	for idx, task := range plan.Tasks {
		id := CanonicalTaskID(task, idx)
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

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

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
