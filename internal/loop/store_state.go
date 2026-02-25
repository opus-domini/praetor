package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) findStateFile(planFile string) (string, bool, error) {
	v2 := s.stateFileV2(planFile)
	if _, err := os.Stat(v2); err == nil {
		return v2, false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat state file: %w", err)
	}

	legacy := s.stateFileLegacy(planFile)
	if _, err := os.Stat(legacy); err == nil {
		return legacy, true, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat legacy state file: %w", err)
	}
	return v2, false, nil
}

func (s *Store) migrateStateFileIfNeeded(planFile, source string, state State) error {
	target := s.stateFileV2(planFile)
	if source == target {
		return nil
	}
	if err := writeJSONFile(target, state); err != nil {
		return fmt.Errorf("migrate legacy state file: %w", err)
	}
	_ = os.Remove(source)
	return nil
}

// LoadOrInitializeState creates state from plan or merges existing state after plan changes.
func (s *Store) LoadOrInitializeState(planFile string, plan Plan) (State, error) {
	if err := s.Init(); err != nil {
		return State{}, err
	}

	checksum, err := PlanChecksum(planFile)
	if err != nil {
		return State{}, err
	}

	stateFile, legacy, err := s.findStateFile(planFile)
	if err != nil {
		return State{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		state := State{
			PlanFile:     planFile,
			PlanChecksum: checksum,
			CreatedAt:    now,
			UpdatedAt:    now,
			Tasks:        stateTasksFromPlan(plan),
		}
		if err := writeJSONFile(s.StateFile(planFile), state); err != nil {
			return State{}, err
		}
		return state, nil
	} else if err != nil {
		return State{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(planFile)
	if err != nil {
		return State{}, err
	}

	if state.PlanChecksum == checksum {
		if legacy {
			if migrateErr := s.migrateStateFileIfNeeded(planFile, stateFile, state); migrateErr != nil {
				return State{}, migrateErr
			}
		}
		return state, nil
	}

	merged := mergeState(planFile, checksum, state, plan)
	if err := writeJSONFile(s.StateFile(planFile), merged); err != nil {
		return State{}, err
	}
	if legacy {
		_ = os.Remove(stateFile)
	}
	return merged, nil
}

// ReadState reads state from disk.
func (s *Store) ReadState(planFile string) (State, error) {
	path, legacy, err := s.findStateFile(planFile)
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode state file: %w", err)
	}
	if legacy {
		if migrateErr := s.migrateStateFileIfNeeded(planFile, path, state); migrateErr != nil {
			return State{}, migrateErr
		}
	}
	return state, nil
}

// WriteState persists state atomically.
func (s *Store) WriteState(planFile string, state State) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeJSONFile(s.StateFile(planFile), state)
}

func stateTasksFromPlan(plan Plan) []StateTask {
	tasks := make([]StateTask, 0, len(plan.Tasks))
	for i, task := range plan.Tasks {
		tasks = append(tasks, StateTask{
			ID:          canonicalTaskID(task, i),
			Title:       strings.TrimSpace(task.Title),
			DependsOn:   normalizedDependsOn(task.DependsOn),
			Executor:    normalizeAgent(task.Executor),
			Reviewer:    normalizeAgent(task.Reviewer),
			Model:       strings.TrimSpace(task.Model),
			Description: strings.TrimSpace(task.Description),
			Criteria:    strings.TrimSpace(task.Criteria),
			Status:      TaskStatusOpen,
		})
	}
	return tasks
}

func normalizedDependsOn(dependsOn []string) []string {
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

func mergeState(planFile, checksum string, previous State, plan Plan) State {
	statusByID := make(map[string]TaskStatus, len(previous.Tasks))
	statusByAutoFingerprint := make(map[string]TaskStatus)
	for _, task := range previous.Tasks {
		statusByID[task.ID] = task.Status
		if strings.HasPrefix(task.ID, "auto-") {
			statusByAutoFingerprint[autoTaskFingerprint(
				task.Title,
				task.Executor,
				task.Reviewer,
				task.Model,
				task.Description,
				task.Criteria,
				task.DependsOn,
			)] = task.Status
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	merged := State{
		PlanFile:     planFile,
		PlanChecksum: checksum,
		CreatedAt:    previous.CreatedAt,
		UpdatedAt:    now,
		Tasks:        stateTasksFromPlan(plan),
	}
	if merged.CreatedAt == "" {
		merged.CreatedAt = now
	}

	for i, task := range merged.Tasks {
		if status, ok := statusByID[task.ID]; ok {
			merged.Tasks[i].Status = status
			continue
		}
		if strings.HasPrefix(task.ID, "auto-") {
			if status, ok := statusByAutoFingerprint[autoTaskFingerprint(
				task.Title,
				task.Executor,
				task.Reviewer,
				task.Model,
				task.Description,
				task.Criteria,
				task.DependsOn,
			)]; ok {
				merged.Tasks[i].Status = status
			}
		}
	}
	return merged
}

func writeJSONFile(path string, value any) error {
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

// DetectStuckTasks reports open tasks that already reached retry limit.
func (s *Store) DetectStuckTasks(planFile string, state State, maxRetries int) ([]string, error) {
	if maxRetries <= 0 {
		return nil, nil
	}

	report := make([]string, 0)
	for idx, task := range state.Tasks {
		if task.Status != TaskStatusOpen {
			continue
		}
		signature := s.TaskSignatureForPlan(planFile, idx, task)
		retries, err := s.ReadRetryCount(signature)
		if err != nil {
			return nil, err
		}
		if retries >= maxRetries {
			report = append(report, fmt.Sprintf("%s: %s (%d/%d retries)", task.ID, task.Title, retries, maxRetries))
		}
	}
	return report, nil
}

// ResetPlanRuntime removes state, lock, retries and feedback for plan tasks.
func (s *Store) ResetPlanRuntime(planFile string, plan Plan) (int, error) {
	removed := 0
	for _, statePath := range []string{s.stateFileV2(planFile), s.stateFileLegacy(planFile)} {
		if err := os.Remove(statePath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove state file: %w", err)
		}
	}

	for _, lockPath := range s.lockCandidates(planFile) {
		if err := os.Remove(lockPath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove lock file: %w", err)
		}
	}

	stateTasks := stateTasksFromPlan(plan)
	for idx, task := range stateTasks {
		signature := s.TaskSignatureForPlan(planFile, idx, task)
		retryPath := filepath.Join(s.RetriesDir(), signature+".count")
		if err := os.Remove(retryPath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove retry file: %w", err)
		}

		feedbackPath := filepath.Join(s.FeedbackDir(), signature+".txt")
		if err := os.Remove(feedbackPath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove feedback file: %w", err)
		}
	}

	return removed, nil
}

// ListStates returns all known state files.
func (s *Store) ListStates() ([]string, error) {
	glob := filepath.Join(s.StateDir(), "*.state.json")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, fmt.Errorf("list state files: %w", err)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return matches, nil
}
