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

// LoadOrInitializeState creates state from plan or merges existing state after plan changes.
func (s *Store) LoadOrInitializeState(slug string, plan domain.Plan) (domain.State, error) {
	if err := s.Init(); err != nil {
		return domain.State{}, err
	}

	planFile := s.PlanFile(slug)
	checksum, err := domain.PlanChecksum(planFile)
	if err != nil {
		return domain.State{}, err
	}

	stateFile := s.StateFile(slug)
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		state := domain.State{
			PlanSlug:        slug,
			PlanChecksum:    checksum,
			CreatedAt:       now,
			UpdatedAt:       now,
			ExecutionPolicy: plan.Settings.ExecutionPolicy,
			Tasks:           domain.StateTasksFromPlan(plan),
		}
		if err := domain.WriteJSONFile(stateFile, state); err != nil {
			return domain.State{}, err
		}
		return state, nil
	} else if err != nil {
		return domain.State{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(slug)
	if err != nil {
		return domain.State{}, err
	}

	migrated := s.normalizeTaskStatuses(slug, &state)
	if state.PlanChecksum == checksum {
		if migrated {
			if err := domain.WriteJSONFile(stateFile, state); err != nil {
				return domain.State{}, err
			}
		}
		return state, nil
	}

	merged := mergeState(slug, checksum, state, plan)
	if err := domain.WriteJSONFile(stateFile, merged); err != nil {
		return domain.State{}, err
	}
	return merged, nil
}

// ReadState reads state from disk.
func (s *Store) ReadState(slug string) (domain.State, error) {
	data, err := os.ReadFile(s.StateFile(slug))
	if err != nil {
		return domain.State{}, fmt.Errorf("read state file: %w", err)
	}
	var state domain.State
	if err := json.Unmarshal(data, &state); err != nil {
		return domain.State{}, fmt.Errorf("decode state file: %w", err)
	}
	return state, nil
}

// WriteState persists state atomically.
func (s *Store) WriteState(slug string, state domain.State) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return domain.WriteJSONFile(s.StateFile(slug), state)
}

func mergeState(slug, checksum string, previous domain.State, plan domain.Plan) domain.State {
	statusByID := make(map[string]domain.TaskStatus, len(previous.Tasks))
	statusByAutoFingerprint := make(map[string]domain.TaskStatus)
	attemptByID := make(map[string]int, len(previous.Tasks))
	feedbackByID := make(map[string]string, len(previous.Tasks))
	costByID := make(map[string]int64, len(previous.Tasks))
	for _, task := range previous.Tasks {
		statusByID[task.ID] = task.Status
		attemptByID[task.ID] = task.Attempt
		feedbackByID[task.ID] = task.Feedback
		costByID[task.ID] = task.CostMicros
		if strings.HasPrefix(task.ID, "auto-") {
			statusByAutoFingerprint[domain.AutoTaskFingerprint(
				task.Title,
				task.Description,
				task.Acceptance,
				task.DependsOn,
			)] = task.Status
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	merged := domain.State{
		PlanSlug:           slug,
		PlanChecksum:       checksum,
		CreatedAt:          previous.CreatedAt,
		UpdatedAt:          now,
		Outcome:            previous.Outcome,
		ExecutionPolicy:    previous.ExecutionPolicy,
		TotalCostMicros:    previous.TotalCostMicros,
		CostWarningEmitted: previous.CostWarningEmitted,
		Tasks:              domain.StateTasksFromPlan(plan),
	}
	if merged.CreatedAt == "" {
		merged.CreatedAt = now
	}
	if merged.ExecutionPolicy == (domain.ExecutionPolicy{}) {
		merged.ExecutionPolicy = plan.Settings.ExecutionPolicy
	}

	for i, task := range merged.Tasks {
		if status, ok := statusByID[task.ID]; ok {
			merged.Tasks[i].Status = status
			merged.Tasks[i].Attempt = attemptByID[task.ID]
			merged.Tasks[i].Feedback = feedbackByID[task.ID]
			merged.Tasks[i].CostMicros = costByID[task.ID]
			continue
		}
		if strings.HasPrefix(task.ID, "auto-") {
			if status, ok := statusByAutoFingerprint[domain.AutoTaskFingerprint(
				task.Title,
				task.Description,
				task.Acceptance,
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

// normalizeTaskStatuses normalizes statuses and absorbs external retry/feedback
// files into StateTask fields. Returns true if any task was modified.
func (s *Store) normalizeTaskStatuses(slug string, state *domain.State) bool {
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
			sig := s.TaskSignatureForPlan(slug, i, *task)
			if count, err := s.ReadRetryCount(sig); err == nil && count > 0 {
				task.Attempt = count
				changed = true
			}
			if task.Feedback == "" {
				if history, err := s.LoadTaskFeedback(slug, sig); err == nil && len(history) > 0 {
					task.Feedback = history[len(history)-1].Reason
					changed = true
				}
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
func (s *Store) ResetPlanRuntime(slug string, plan domain.Plan) (int, error) {
	removed := 0
	if err := os.Remove(s.StateFile(slug)); err == nil {
		removed++
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return removed, fmt.Errorf("remove state file: %w", err)
	}

	if err := os.Remove(s.LockFile(slug)); err == nil {
		removed++
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return removed, fmt.Errorf("remove lock file: %w", err)
	}

	stateTasks := domain.StateTasksFromPlan(plan)
	for idx, task := range stateTasks {
		signature := s.TaskSignatureForPlan(slug, idx, task)
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

		structuredFeedbackPath := filepath.Join(s.FeedbackDir(), slug, signature+".jsonl")
		if err := os.Remove(structuredFeedbackPath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove structured feedback file: %w", err)
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
