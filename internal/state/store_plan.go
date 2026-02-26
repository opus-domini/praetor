package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// Status returns current execution status for a plan.
func (s *Store) Status(planFile string) (domain.PlanStatus, error) {
	planFile = strings.TrimSpace(planFile)
	if planFile == "" {
		return domain.PlanStatus{}, errors.New("plan file is required")
	}

	if _, err := os.Stat(planFile); err != nil {
		return domain.PlanStatus{}, fmt.Errorf("plan file not found: %w", err)
	}

	// Read plan file inline to get task count (avoids importing loop.LoadPlan).
	planData, err := os.ReadFile(planFile)
	if err != nil {
		return domain.PlanStatus{}, fmt.Errorf("read plan file: %w", err)
	}
	var plan domain.Plan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return domain.PlanStatus{}, fmt.Errorf("decode plan file: %w", err)
	}

	stateFile, _, err := s.findStateFile(planFile)
	if err != nil {
		return domain.PlanStatus{}, err
	}
	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		return domain.PlanStatus{
			PlanFile: planFile,
			Total:    len(plan.Tasks),
			Active:   len(plan.Tasks),
			Done:     0,
		}, nil
	} else if err != nil {
		return domain.PlanStatus{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(planFile)
	if err != nil {
		return domain.PlanStatus{}, err
	}
	lockRunning, _ := s.IsPlanRunning(planFile)

	return domain.PlanStatus{
		PlanFile:  planFile,
		StateFile: stateFile,
		UpdatedAt: state.UpdatedAt,
		Done:      state.DoneCount(),
		Failed:    state.FailedCount(),
		Active:    state.ActiveCount(),
		Total:     len(state.Tasks),
		Running:   lockRunning,
		Tasks:     state.Tasks,
	}, nil
}

// ListPlanStatuses returns state summaries for every known state file.
func (s *Store) ListPlanStatuses() ([]domain.PlanStatus, error) {
	files, err := s.ListStates()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	statuses := make([]domain.PlanStatus, 0, len(files))
	for _, stateFile := range files {
		data, readErr := os.ReadFile(stateFile)
		if readErr != nil {
			return nil, fmt.Errorf("read state file: %w", readErr)
		}

		state := domain.State{}
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("decode state file: %w", err)
		}

		planFile := strings.TrimSpace(state.PlanFile)
		if planFile == "" {
			planFile = inferPlanFromState(stateFile)
		}

		running, _ := s.IsPlanRunning(planFile)
		statuses = append(statuses, domain.PlanStatus{
			PlanFile:  planFile,
			StateFile: stateFile,
			UpdatedAt: state.UpdatedAt,
			Done:      state.DoneCount(),
			Failed:    state.FailedCount(),
			Active:    state.ActiveCount(),
			Total:     len(state.Tasks),
			Running:   running,
		})
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].PlanFile < statuses[j].PlanFile
	})
	return statuses, nil
}

func inferPlanFromState(stateFile string) string {
	base := filepath.Base(stateFile)
	base = strings.TrimSuffix(base, ".state.json")
	if idx := strings.LastIndex(base, "--"); idx > 0 {
		base = base[:idx]
	}
	return base + ".json"
}

// IsPlanRunning reports whether the lock PID is alive.
func (s *Store) IsPlanRunning(planFile string) (bool, int) {
	hostname, _ := os.Hostname()
	runtimeKey := s.RuntimeKey(planFile)
	for _, lockPath := range s.lockCandidates(planFile) {
		data, err := os.ReadFile(lockPath)
		if err != nil {
			continue
		}
		meta := parseLockFile(data)
		if meta.PID <= 0 {
			continue
		}
		if lockIsActive(meta, hostname, runtimeKey) {
			return true, meta.PID
		}
	}
	return false, 0
}
