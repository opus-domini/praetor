package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PlanStatus describes execution status of a plan.
type PlanStatus struct {
	PlanFile  string
	StateFile string
	UpdatedAt string
	Done      int
	Open      int
	Total     int
	Running   bool
	Tasks     []StateTask
}

// Status returns current execution status for a plan.
func (s *Store) Status(planFile string) (PlanStatus, error) {
	planFile = strings.TrimSpace(planFile)
	if planFile == "" {
		return PlanStatus{}, errors.New("plan file is required")
	}

	if _, err := os.Stat(planFile); err != nil {
		return PlanStatus{}, fmt.Errorf("plan file not found: %w", err)
	}

	plan, err := LoadPlan(planFile)
	if err != nil {
		return PlanStatus{}, err
	}

	stateFile := s.StateFile(planFile)
	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		return PlanStatus{
			PlanFile: planFile,
			Total:    len(plan.Tasks),
			Open:     len(plan.Tasks),
			Done:     0,
		}, nil
	} else if err != nil {
		return PlanStatus{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(planFile)
	if err != nil {
		return PlanStatus{}, err
	}
	lockRunning, _ := s.IsPlanRunning(planFile)

	return PlanStatus{
		PlanFile:  planFile,
		StateFile: stateFile,
		UpdatedAt: state.UpdatedAt,
		Done:      state.DoneCount(),
		Open:      state.OpenCount(),
		Total:     len(state.Tasks),
		Running:   lockRunning,
		Tasks:     state.Tasks,
	}, nil
}

// ListPlanStatuses returns state summaries for every known state file.
func (s *Store) ListPlanStatuses() ([]PlanStatus, error) {
	files, err := s.ListStates()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}

	statuses := make([]PlanStatus, 0, len(files))
	for _, stateFile := range files {
		data, readErr := os.ReadFile(stateFile)
		if readErr != nil {
			return nil, fmt.Errorf("read state file: %w", readErr)
		}

		state := State{}
		if err := json.Unmarshal(data, &state); err != nil {
			return nil, fmt.Errorf("decode state file: %w", err)
		}

		planFile := strings.TrimSpace(state.PlanFile)
		if planFile == "" {
			planFile = inferPlanFromState(stateFile)
		}

		running, _ := s.IsPlanRunning(planFile)
		statuses = append(statuses, PlanStatus{
			PlanFile:  planFile,
			StateFile: stateFile,
			UpdatedAt: state.UpdatedAt,
			Done:      state.DoneCount(),
			Open:      state.OpenCount(),
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
	return base + ".json"
}

// IsPlanRunning reports whether the lock PID is alive.
func (s *Store) IsPlanRunning(planFile string) (bool, int) {
	data, err := os.ReadFile(s.LockFile(planFile))
	if err != nil {
		return false, 0
	}
	pid, _ := parseLockFile(data)
	if pid <= 0 {
		return false, 0
	}
	return processIsRunning(pid), pid
}
