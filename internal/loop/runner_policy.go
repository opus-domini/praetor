package loop

import (
	"fmt"
	"time"
)

type TransitionRecorder struct {
	store    *Store
	planFile string
}

func NewTransitionRecorder(store *Store, planFile string) *TransitionRecorder {
	return &TransitionRecorder{store: store, planFile: planFile}
}

func (r *TransitionRecorder) WriteCheckpoint(entry CheckpointEntry) {
	_ = r.store.WriteCheckpoint(r.planFile, entry)
}

func (r *TransitionRecorder) WriteMetric(entry CostEntry) {
	_ = r.store.WriteTaskMetrics(entry)
}

func (r *TransitionRecorder) RetryTask(signature, feedback string) (int, error) {
	nextRetry, err := r.store.IncrementRetryCount(signature)
	if err != nil {
		return 0, err
	}
	if feedback != "" {
		_ = r.store.WriteFeedback(signature, feedback)
	}
	return nextRetry, nil
}

func (r *TransitionRecorder) CompleteTask(state *State, index int, signature, runID, message string) error {
	state.Tasks[index].Status = TaskStatusDone
	if err := r.store.WriteState(r.planFile, *state); err != nil {
		return err
	}
	if err := r.store.ClearRetryCount(signature); err != nil {
		return err
	}
	if err := r.store.ClearFeedback(signature); err != nil {
		return err
	}
	r.WriteCheckpoint(CheckpointEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    "completed",
		TaskID:    state.Tasks[index].ID,
		Signature: signature,
		RunID:     runID,
		Message:   message,
	})
	return nil
}

type GitSafetyPolicy struct {
	store   *Store
	workdir string
	enabled bool
}

func NewGitSafetyPolicy(store *Store, workdir string, enabled bool) *GitSafetyPolicy {
	return &GitSafetyPolicy{store: store, workdir: workdir, enabled: enabled}
}

func (p *GitSafetyPolicy) Enabled() bool {
	return p.enabled
}

func (p *GitSafetyPolicy) PrepareTask(runID string) error {
	if !p.enabled {
		return nil
	}
	return p.store.SaveGitSnapshot(runID, p.workdir)
}

func (p *GitSafetyPolicy) RollbackTask(runID string, render *Renderer) {
	if !p.enabled {
		return
	}
	if err := p.store.RollbackGitSnapshot(runID, p.workdir); err != nil {
		render.Warn(fmt.Sprintf("git rollback failed: %v", err))
	}
}

func (p *GitSafetyPolicy) CommitTask(runID string) {
	if !p.enabled {
		return
	}
	_ = p.store.DiscardGitSnapshot(runID)
}
