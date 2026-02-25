package loop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TransitionRecorder centralizes state transition side effects.
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

// worktreeInfo tracks one active worktree.
type worktreeInfo struct {
	path   string
	branch string
}

// IsolationPolicy controls how tasks are isolated from the main working tree.
type IsolationPolicy struct {
	mainDir string
	mode    IsolationMode
	active  map[string]worktreeInfo
}

// NewIsolationPolicy creates an isolation policy.
func NewIsolationPolicy(mainDir string, mode IsolationMode) *IsolationPolicy {
	return &IsolationPolicy{
		mainDir: mainDir,
		mode:    mode,
		active:  make(map[string]worktreeInfo),
	}
}

// PrepareTask creates an isolated worktree for one task execution.
// Returns the workdir the agent should use.
func (p *IsolationPolicy) PrepareTask(runID, taskID string) (string, error) {
	if p.mode == IsolationOff {
		return p.mainDir, nil
	}

	branch := fmt.Sprintf("praetor/%s--%s", sanitizePathToken(taskID), runID[:8])
	worktreePath := filepath.Join(p.mainDir, ".praetor", "worktrees", fmt.Sprintf("%s--%s", sanitizePathToken(taskID), runID[:8]))

	addCmd := exec.Command("git", "-C", p.mainDir, "worktree", "add", worktreePath, "-b", branch, "HEAD")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Initialize submodules if present.
	if _, err := os.Stat(filepath.Join(p.mainDir, ".gitmodules")); err == nil {
		subCmd := exec.Command("git", "-C", worktreePath, "submodule", "update", "--init", "--recursive")
		_ = subCmd.Run()
	}

	p.active[runID] = worktreeInfo{path: worktreePath, branch: branch}
	return worktreePath, nil
}

// CommitTask auto-commits changes in the worktree, merges into main, and cleans up.
func (p *IsolationPolicy) CommitTask(runID string) error {
	info, ok := p.active[runID]
	if !ok {
		return nil
	}
	defer p.removeWorktree(runID)

	// Auto-commit if executor left uncommitted changes.
	addCmd := exec.Command("git", "-C", info.path, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A in worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	diffCmd := exec.Command("git", "-C", info.path, "diff", "--cached", "--quiet")
	if diffCmd.Run() != nil {
		// There are staged changes — commit them.
		commitCmd := exec.Command("git", "-C", info.path, "commit", "-m", fmt.Sprintf("praetor: %s", runID))
		if out, err := commitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("auto-commit in worktree: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}

	// Merge worktree branch into main.
	mergeCmd := exec.Command("git", "-C", p.mainDir, "merge", "--no-edit", info.branch)
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		// Abort the failed merge to leave main clean.
		_ = exec.Command("git", "-C", p.mainDir, "merge", "--abort").Run()
		return fmt.Errorf("merge conflict: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RollbackTask removes the worktree and branch without merging.
func (p *IsolationPolicy) RollbackTask(runID string, render *Renderer) {
	if _, ok := p.active[runID]; !ok {
		return
	}
	p.removeWorktree(runID)
}

// Cleanup removes all active worktrees. Called from deferred cleanup.
func (p *IsolationPolicy) Cleanup() {
	for runID := range p.active {
		p.removeWorktree(runID)
	}
}

// PruneOrphans cleans up worktree metadata from previous crashes.
func (p *IsolationPolicy) PruneOrphans() {
	if p.mode == IsolationOff {
		return
	}
	_ = exec.Command("git", "-C", p.mainDir, "worktree", "prune").Run()
}

func (p *IsolationPolicy) removeWorktree(runID string) {
	info, ok := p.active[runID]
	if !ok {
		return
	}
	delete(p.active, runID)

	_ = exec.Command("git", "-C", p.mainDir, "worktree", "remove", "--force", info.path).Run()
	_ = exec.Command("git", "-C", p.mainDir, "branch", "-D", info.branch).Run()
}
