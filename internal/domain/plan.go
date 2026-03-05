package domain

import (
	"bytes"
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

var supportedQualityCommandGates = map[string]struct{}{
	"tests":     {},
	"lint":      {},
	"standards": {},
}

// CanonicalTaskID returns the stable identifier for a plan task.
func CanonicalTaskID(task Task, _ int) string {
	return strings.TrimSpace(task.ID)
}

// AutoTaskFingerprint builds a stable task fingerprint used for state merges.
func AutoTaskFingerprint(title, description string, acceptance, dependsOn []string) string {
	normalizedAcceptance := make([]string, 0, len(acceptance))
	for _, item := range acceptance {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		normalizedAcceptance = append(normalizedAcceptance, item)
	}

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
		strings.TrimSpace(description),
		strings.Join(normalizedAcceptance, "\n"),
		strings.Join(normalizedDeps, ","),
	}
	return strings.Join(parts, "\n")
}

// StateTasksFromPlan creates state tasks from plan tasks.
func StateTasksFromPlan(plan Plan) []StateTask {
	tasks := make([]StateTask, 0, len(plan.Tasks))
	for i, task := range plan.Tasks {
		id := CanonicalTaskID(task, i)
		if id == "" {
			id = fmt.Sprintf("TASK-%03d", i+1)
		}
		tasks = append(tasks, StateTask{
			ID:          id,
			Title:       strings.TrimSpace(task.Title),
			DependsOn:   NormalizedDependsOn(task.DependsOn),
			Description: strings.TrimSpace(task.Description),
			Acceptance:  normalizeAcceptance(task.Acceptance),
			Status:      TaskPending,
		})
	}
	return tasks
}

func normalizeAcceptance(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil
	}
	return result
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

// LoadPlan reads and validates a plan file with strict unknown-field checks.
func LoadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("read plan file: %w", err)
	}

	plan, err := decodePlanStrict(data)
	if err != nil {
		return Plan{}, err
	}
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// ParsePlanLenient decodes planner output, ignoring unknown fields.
func ParsePlanLenient(data []byte) (Plan, error) {
	plan := Plan{}
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, fmt.Errorf("decode plan json: %w", err)
	}
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// ParsePlanStrict decodes planner output with strict unknown-field checks.
func ParsePlanStrict(data []byte) (Plan, error) {
	plan, err := decodePlanStrict(data)
	if err != nil {
		return Plan{}, err
	}
	if err := ValidatePlan(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func decodePlanStrict(data []byte) (Plan, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var plan Plan
	if err := dec.Decode(&plan); err != nil {
		return Plan{}, fmt.Errorf("decode plan file: %w", err)
	}
	if dec.More() {
		return Plan{}, errors.New("decode plan file: multiple JSON documents are not allowed")
	}
	return plan, nil
}

// ValidatePlan validates logical constraints for a plan.
func ValidatePlan(plan Plan) error {
	errorsList := make([]string, 0)

	if strings.TrimSpace(plan.Name) == "" {
		errorsList = append(errorsList, "name is required")
	}
	if len(plan.Tasks) == 0 {
		errorsList = append(errorsList, "tasks array cannot be empty")
	}

	executor := NormalizeAgent(plan.Settings.Agents.Executor.Agent)
	if executor == "" {
		errorsList = append(errorsList, "settings.agents.executor.agent is required")
	} else if _, ok := ValidExecutors[executor]; !ok {
		errorsList = append(errorsList, fmt.Sprintf("settings.agents.executor.agent has invalid value %q", plan.Settings.Agents.Executor.Agent))
	}

	reviewer := NormalizeAgent(plan.Settings.Agents.Reviewer.Agent)
	if reviewer == "" {
		errorsList = append(errorsList, "settings.agents.reviewer.agent is required")
	} else if _, ok := ValidReviewers[reviewer]; !ok {
		errorsList = append(errorsList, fmt.Sprintf("settings.agents.reviewer.agent has invalid value %q", plan.Settings.Agents.Reviewer.Agent))
	}

	planner := NormalizeAgent(plan.Settings.Agents.Planner.Agent)
	if planner != "" {
		if _, ok := ValidExecutors[planner]; !ok {
			errorsList = append(errorsList, fmt.Sprintf("settings.agents.planner.agent has invalid value %q", plan.Settings.Agents.Planner.Agent))
		}
	}

	if strings.TrimSpace(plan.Settings.ExecutionPolicy.Timeout) != "" {
		if _, err := time.ParseDuration(strings.TrimSpace(plan.Settings.ExecutionPolicy.Timeout)); err != nil {
			errorsList = append(errorsList, fmt.Sprintf("settings.execution_policy.timeout has invalid duration %q", plan.Settings.ExecutionPolicy.Timeout))
		}
	}
	if plan.Settings.ExecutionPolicy.MaxTotalIterations < 0 {
		errorsList = append(errorsList, "settings.execution_policy.max_total_iterations cannot be negative")
	}
	if plan.Settings.ExecutionPolicy.MaxRetriesPerTask < 0 {
		errorsList = append(errorsList, "settings.execution_policy.max_retries_per_task cannot be negative")
	}
	if plan.Settings.ExecutionPolicy.Budget.Execute < 0 {
		errorsList = append(errorsList, "settings.execution_policy.budget.execute cannot be negative")
	}
	if plan.Settings.ExecutionPolicy.Budget.Review < 0 {
		errorsList = append(errorsList, "settings.execution_policy.budget.review cannot be negative")
	}
	if plan.Settings.ExecutionPolicy.StallDetection.Window < 0 {
		errorsList = append(errorsList, "settings.execution_policy.stall_detection.window cannot be negative")
	}
	if plan.Settings.ExecutionPolicy.StallDetection.Threshold < 0 || plan.Settings.ExecutionPolicy.StallDetection.Threshold > 1 {
		errorsList = append(errorsList, "settings.execution_policy.stall_detection.threshold must be between 0 and 1")
	}

	ids := make(map[string]int, len(plan.Tasks))
	for idx, task := range plan.Tasks {
		id := CanonicalTaskID(task, idx)
		if id == "" {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: id is required", idx))
		} else if prev, exists := ids[id]; exists {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: duplicated id %q already used by tasks[%d]", idx, id, prev))
		} else {
			ids[id] = idx
		}

		if strings.TrimSpace(task.Title) == "" {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: title is required", idx))
		}
		if len(normalizeAcceptance(task.Acceptance)) == 0 {
			errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: acceptance must contain at least one item", idx))
		}
		for _, dep := range task.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: depends_on contains an empty id", idx))
				continue
			}
			if dep == id && id != "" {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: depends_on cannot reference itself (%q)", idx, dep))
			}
		}
		if task.Constraints != nil && strings.TrimSpace(task.Constraints.Timeout) != "" {
			if _, err := time.ParseDuration(strings.TrimSpace(task.Constraints.Timeout)); err != nil {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d].constraints.timeout has invalid duration %q", idx, task.Constraints.Timeout))
			}
		}
		if task.Agents != nil {
			if task.Agents.Executor != "" {
				exec := NormalizeAgent(Agent(task.Agents.Executor))
				if _, ok := ValidExecutors[exec]; !ok {
					errorsList = append(errorsList, fmt.Sprintf("tasks[%d].agents.executor has invalid value %q", idx, task.Agents.Executor))
				}
			}
			if task.Agents.Reviewer != "" {
				rev := NormalizeAgent(Agent(task.Agents.Reviewer))
				if _, ok := ValidReviewers[rev]; !ok {
					errorsList = append(errorsList, fmt.Sprintf("tasks[%d].agents.reviewer has invalid value %q", idx, task.Agents.Reviewer))
				}
			}
		}
	}

	for gate, command := range plan.Quality.Commands {
		name := strings.ToLower(strings.TrimSpace(gate))
		if _, ok := supportedQualityCommandGates[name]; !ok {
			errorsList = append(errorsList, fmt.Sprintf("quality.commands contains unsupported gate %q (allowed: tests, lint, standards)", gate))
			continue
		}
		if strings.TrimSpace(command) == "" {
			errorsList = append(errorsList, fmt.Sprintf("quality.commands.%s cannot be empty", name))
		}
	}

	for idx, task := range plan.Tasks {
		for _, dep := range task.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if _, ok := ids[dep]; !ok {
				errorsList = append(errorsList, fmt.Sprintf("tasks[%d]: depends_on references unknown task id %q", idx, dep))
			}
		}
	}

	if cycle := findCycle(plan.Tasks); len(cycle) > 0 {
		errorsList = append(errorsList, fmt.Sprintf("tasks dependency graph contains cycle: %s", strings.Join(cycle, " -> ")))
	}

	if len(errorsList) == 0 {
		return nil
	}
	sort.Strings(errorsList)
	return errors.New("plan validation failed:\n- " + strings.Join(errorsList, "\n- "))
}

func findCycle(tasks []Task) []string {
	deps := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		id := strings.TrimSpace(task.ID)
		if id == "" {
			continue
		}
		deps[id] = NormalizedDependsOn(task.DependsOn)
	}

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(deps))
	stack := make([]string, 0, len(deps))
	indexInStack := make(map[string]int, len(deps))

	var dfs func(node string) []string
	dfs = func(node string) []string {
		state[node] = visiting
		indexInStack[node] = len(stack)
		stack = append(stack, node)

		for _, dep := range deps[node] {
			if _, ok := deps[dep]; !ok {
				continue
			}
			switch state[dep] {
			case unvisited:
				if cycle := dfs(dep); len(cycle) > 0 {
					return cycle
				}
			case visiting:
				start := indexInStack[dep]
				cycle := append([]string{}, stack[start:]...)
				cycle = append(cycle, dep)
				return cycle
			}
		}

		stack = stack[:len(stack)-1]
		delete(indexInStack, node)
		state[node] = done
		return nil
	}

	ids := make([]string, 0, len(deps))
	for id := range deps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if state[id] != unvisited {
			continue
		}
		if cycle := dfs(id); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
var nonSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)
var multiDashPattern = regexp.MustCompile(`-+`)

// Slugify converts an arbitrary name to a slug candidate.
func Slugify(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "õ", "o", "ô", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
	).Replace(name)
	name = nonSlugPattern.ReplaceAllString(name, "-")
	name = multiDashPattern.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "plan"
	}
	return name
}

// NextAvailableSlug resolves a unique slug in plansDir by adding -2, -3, ... when needed.
func NextAvailableSlug(plansDir, base string) (string, error) {
	base = strings.TrimSpace(base)
	if !slugPattern.MatchString(base) {
		return "", fmt.Errorf("invalid slug %q (allowed: lowercase letters, digits, hyphens)", base)
	}
	path := filepath.Join(plansDir, base+".json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		path = filepath.Join(plansDir, candidate+".json")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to allocate unique slug for %q", base)
}

// NewPlanFile creates a skeleton plan file in the given plans directory.
func NewPlanFile(slug, plansDir string) (string, error) {
	slug = strings.TrimSpace(slug)
	if !slugPattern.MatchString(slug) {
		return "", fmt.Errorf("invalid slug %q (allowed: lowercase letters, digits, hyphens)", slug)
	}

	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return "", fmt.Errorf("create plans directory: %w", err)
	}

	path := filepath.Join(plansDir, slug+".json")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("plan file already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check plan file: %w", err)
	}

	plan := Plan{
		Name:    strings.ReplaceAll(slug, "-", " "),
		Summary: "TODO: describe the goal of this plan.",
		Meta: PlanMeta{
			Source:    "manual",
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		},
		Settings: PlanSettings{
			Agents: PlanAgents{
				Executor: PlanAgentConfig{Agent: AgentCodex},
				Reviewer: PlanAgentConfig{Agent: AgentClaude},
			},
		},
		Tasks: []Task{
			{
				ID:          "TASK-001",
				Title:       "First task",
				Description: "TODO: describe what this task should do.",
				Acceptance: []string{
					"Define objective acceptance criteria.",
				},
			},
			{
				ID:          "TASK-002",
				Title:       "Second task",
				DependsOn:   []string{"TASK-001"},
				Description: "TODO: describe what this task should do.",
				Acceptance: []string{
					"Define objective acceptance criteria.",
				},
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
