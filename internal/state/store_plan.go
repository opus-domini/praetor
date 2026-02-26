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

// Status returns current execution status for a plan slug.
func (s *Store) Status(slug string) (domain.PlanStatus, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return domain.PlanStatus{}, errors.New("plan slug is required")
	}

	planFile := s.PlanFile(slug)
	if _, err := os.Stat(planFile); err != nil {
		return domain.PlanStatus{}, fmt.Errorf("plan file not found: %w", err)
	}

	planData, err := os.ReadFile(planFile)
	if err != nil {
		return domain.PlanStatus{}, fmt.Errorf("read plan file: %w", err)
	}
	var plan domain.Plan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return domain.PlanStatus{}, fmt.Errorf("decode plan file: %w", err)
	}

	stateFile := s.StateFile(slug)
	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		return domain.PlanStatus{
			PlanSlug: slug,
			Total:    len(plan.Tasks),
			Active:   len(plan.Tasks),
			Done:     0,
		}, nil
	} else if err != nil {
		return domain.PlanStatus{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(slug)
	if err != nil {
		return domain.PlanStatus{}, err
	}
	lockRunning, _ := s.IsPlanRunning(slug)

	return domain.PlanStatus{
		PlanSlug:  slug,
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

// ListPlanSlugs returns all plan slugs found in the plans directory.
func (s *Store) ListPlanSlugs() ([]string, error) {
	glob := filepath.Join(s.PlansDir(), "*.json")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, fmt.Errorf("list plan files: %w", err)
	}
	slugs := make([]string, 0, len(matches))
	for _, match := range matches {
		base := filepath.Base(match)
		slug := strings.TrimSuffix(base, ".json")
		if slug != "" {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)
	return slugs, nil
}

// ListPlanStatuses returns state summaries for every known plan.
func (s *Store) ListPlanStatuses() ([]domain.PlanStatus, error) {
	slugs, err := s.ListPlanSlugs()
	if err != nil {
		return nil, err
	}
	if len(slugs) == 0 {
		return nil, nil
	}

	statuses := make([]domain.PlanStatus, 0, len(slugs))
	for _, slug := range slugs {
		stateFile := s.StateFile(slug)
		if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
			// Plan exists but has never been run.
			planFile := s.PlanFile(slug)
			planData, readErr := os.ReadFile(planFile)
			if readErr != nil {
				continue
			}
			var plan domain.Plan
			if json.Unmarshal(planData, &plan) != nil {
				continue
			}
			statuses = append(statuses, domain.PlanStatus{
				PlanSlug: slug,
				Total:    len(plan.Tasks),
				Active:   len(plan.Tasks),
			})
			continue
		} else if err != nil {
			continue
		}

		data, readErr := os.ReadFile(stateFile)
		if readErr != nil {
			continue
		}
		state := domain.State{}
		if json.Unmarshal(data, &state) != nil {
			continue
		}

		running, _ := s.IsPlanRunning(slug)
		statuses = append(statuses, domain.PlanStatus{
			PlanSlug:  slug,
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
		return statuses[i].PlanSlug < statuses[j].PlanSlug
	})
	return statuses, nil
}

// IsPlanRunning reports whether the lock PID is alive.
func (s *Store) IsPlanRunning(slug string) (bool, int) {
	hostname, _ := os.Hostname()
	runtimeKey := s.RuntimeKey(slug)
	data, err := os.ReadFile(s.LockFile(slug))
	if err != nil {
		return false, 0
	}
	meta := parseLockFile(data)
	if meta.PID <= 0 {
		return false, 0
	}
	if lockIsActive(meta, hostname, runtimeKey) {
		return true, meta.PID
	}
	return false, 0
}
