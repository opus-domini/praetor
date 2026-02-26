package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
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

func (s *Store) migrateStateFileIfNeeded(planFile, source string, state domain.State) error {
	target := s.stateFileV2(planFile)
	if source == target {
		return nil
	}
	if err := domain.WriteJSONFile(target, state); err != nil {
		return fmt.Errorf("migrate legacy state file: %w", err)
	}
	_ = os.Remove(source)
	return nil
}

// LoadOrInitializeState creates state from plan or merges existing state after plan changes.
func (s *Store) LoadOrInitializeState(planFile string, plan domain.Plan) (domain.State, error) {
	if err := s.Init(); err != nil {
		return domain.State{}, err
	}

	checksum, err := domain.PlanChecksum(planFile)
	if err != nil {
		return domain.State{}, err
	}

	stateFile, legacy, err := s.findStateFile(planFile)
	if err != nil {
		return domain.State{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		state := domain.State{
			PlanFile:     planFile,
			PlanChecksum: checksum,
			CreatedAt:    now,
			UpdatedAt:    now,
			Tasks:        domain.StateTasksFromPlan(plan),
		}
		if err := domain.WriteJSONFile(s.StateFile(planFile), state); err != nil {
			return domain.State{}, err
		}
		return state, nil
	} else if err != nil {
		return domain.State{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(planFile)
	if err != nil {
		return domain.State{}, err
	}

	migrated := s.normalizeTaskStatuses(planFile, &state)
	if state.PlanChecksum == checksum {
		if legacy || migrated {
			if migrateErr := s.migrateStateFileIfNeeded(planFile, stateFile, state); migrateErr != nil {
				return domain.State{}, migrateErr
			}
			if !legacy && migrated {
				if err := domain.WriteJSONFile(s.StateFile(planFile), state); err != nil {
					return domain.State{}, err
				}
			}
		}
		return state, nil
	}

	merged := mergeState(planFile, checksum, state, plan)
	if err := domain.WriteJSONFile(s.StateFile(planFile), merged); err != nil {
		return domain.State{}, err
	}
	if legacy {
		_ = os.Remove(stateFile)
	}
	return merged, nil
}

// ReadState reads state from disk.
func (s *Store) ReadState(planFile string) (domain.State, error) {
	path, legacy, err := s.findStateFile(planFile)
	if err != nil {
		return domain.State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.State{}, fmt.Errorf("read state file: %w", err)
	}
	var state domain.State
	if err := json.Unmarshal(data, &state); err != nil {
		return domain.State{}, fmt.Errorf("decode state file: %w", err)
	}
	if legacy {
		if migrateErr := s.migrateStateFileIfNeeded(planFile, path, state); migrateErr != nil {
			return domain.State{}, migrateErr
		}
	}
	return state, nil
}

// WriteState persists state atomically.
func (s *Store) WriteState(planFile string, state domain.State) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return domain.WriteJSONFile(s.StateFile(planFile), state)
}

func mergeState(planFile, checksum string, previous domain.State, plan domain.Plan) domain.State {
	statusByID := make(map[string]domain.TaskStatus, len(previous.Tasks))
	statusByAutoFingerprint := make(map[string]domain.TaskStatus)
	for _, task := range previous.Tasks {
		statusByID[task.ID] = task.Status
		if strings.HasPrefix(task.ID, "auto-") {
			statusByAutoFingerprint[domain.AutoTaskFingerprint(
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
	merged := domain.State{
		PlanFile:     planFile,
		PlanChecksum: checksum,
		CreatedAt:    previous.CreatedAt,
		UpdatedAt:    now,
		Tasks:        domain.StateTasksFromPlan(plan),
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
			if status, ok := statusByAutoFingerprint[domain.AutoTaskFingerprint(
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

// DetectStuckTasks reports tasks that are in failed state.
func (s *Store) DetectStuckTasks(_ string, state domain.State, _ int) ([]string, error) {
	report := make([]string, 0)
	for _, task := range state.Tasks {
		if task.Status == domain.TaskFailed {
			report = append(report, fmt.Sprintf("%s: %s (failed after %d attempts)", task.ID, task.Title, task.Attempt))
		}
	}
	return report, nil
}

// normalizeTaskStatuses migrates legacy statuses and absorbs external retry/feedback
// files into StateTask fields. Returns true if any task was modified.
func (s *Store) normalizeTaskStatuses(planFile string, state *domain.State) bool {
	changed := false
	for i := range state.Tasks {
		task := &state.Tasks[i]
		normalized := domain.NormalizeStatus(task.Status)
		if normalized != task.Status {
			task.Status = normalized
			changed = true
		}

		// Absorb external retry count if task has no inline attempt count.
		if task.Attempt == 0 && task.Status == domain.TaskPending {
			sig := s.TaskSignatureForPlan(planFile, i, *task)
			if count, err := s.ReadRetryCount(sig); err == nil && count > 0 {
				task.Attempt = count
				changed = true
			}
			if task.Feedback == "" {
				if fb, err := s.ReadFeedback(sig); err == nil && fb != "" {
					task.Feedback = fb
					changed = true
				}
			}
		}
	}
	return changed
}

// ResetPlanRuntime removes state, lock, retries and feedback for plan tasks.
func (s *Store) ResetPlanRuntime(planFile string, plan domain.Plan) (int, error) {
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

	stateTasks := domain.StateTasksFromPlan(plan)
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
