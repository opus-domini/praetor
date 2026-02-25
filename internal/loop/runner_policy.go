package loop

import (
	"context"
	"errors"
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

func (r *TransitionRecorder) WriteCheckpoint(entry CheckpointEntry) error {
	return r.store.WriteCheckpoint(r.planFile, entry)
}

func (r *TransitionRecorder) WriteMetric(entry CostEntry) error {
	return r.store.WriteTaskMetrics(entry)
}

func (r *TransitionRecorder) RetryTask(signature, feedback string) (int, error) {
	nextRetry, err := r.store.IncrementRetryCount(signature)
	if err != nil {
		return 0, err
	}
	if feedback != "" {
		if err := r.store.WriteFeedback(signature, feedback); err != nil {
			return 0, err
		}
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
	return r.WriteCheckpoint(CheckpointEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Status:    "completed",
		TaskID:    state.Tasks[index].ID,
		Signature: signature,
		RunID:     runID,
		Message:   message,
	})
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
func (p *IsolationPolicy) PrepareTask(ctx context.Context, runID, taskID string) (string, error) {
	if p.mode == IsolationOff {
		return p.mainDir, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runToken := shortToken(runID, 8)
	branch := fmt.Sprintf("praetor/%s--%s", sanitizePathToken(taskID), runToken)
	worktreePath := filepath.Join(p.mainDir, ".praetor", "worktrees", fmt.Sprintf("%s--%s", sanitizePathToken(taskID), runToken))

	addCmd := exec.CommandContext(ctx, "git", "-C", p.mainDir, "worktree", "add", worktreePath, "-b", branch, "HEAD")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Initialize submodules if present.
	if _, err := os.Stat(filepath.Join(p.mainDir, ".gitmodules")); err == nil {
		subCmd := exec.CommandContext(ctx, "git", "-C", worktreePath, "submodule", "update", "--init", "--recursive")
		_ = subCmd.Run()
	}

	p.active[runID] = worktreeInfo{path: worktreePath, branch: branch}
	return worktreePath, nil
}

// CommitTask auto-commits changes in the worktree, merges into main, and cleans up.
func (p *IsolationPolicy) CommitTask(ctx context.Context, runID string) error {
	info, ok := p.active[runID]
	if !ok {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer p.removeWorktree(context.WithoutCancel(ctx), runID)

	// Auto-commit if executor left uncommitted changes.
	addCmd := exec.CommandContext(ctx, "git", "-C", info.path, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A in worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	diffCmd := exec.CommandContext(ctx, "git", "-C", info.path, "diff", "--cached", "--quiet")
	diffErr := diffCmd.Run()
	hasStagedChanges := false
	if diffErr != nil {
		var exitErr *exec.ExitError
		if errors.As(diffErr, &exitErr) && exitErr.ExitCode() == 1 {
			hasStagedChanges = true
		} else {
			return fmt.Errorf("check staged changes in worktree: %w", diffErr)
		}
	}
	if hasStagedChanges {
		// There are staged changes — commit them.
		commitCmd := exec.CommandContext(ctx, "git", "-C", info.path, "commit", "-m", fmt.Sprintf("praetor: %s", runID))
		if out, err := commitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("auto-commit in worktree: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}

	// Merge worktree branch into main.
	mergeCmd := exec.CommandContext(ctx, "git", "-C", p.mainDir, "merge", "--no-edit", info.branch)
	if out, err := mergeCmd.CombinedOutput(); err != nil {
		// Abort the failed merge to leave main clean.
		_ = exec.CommandContext(context.WithoutCancel(ctx), "git", "-C", p.mainDir, "merge", "--abort").Run()
		return fmt.Errorf("merge conflict: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RollbackTask removes the worktree and branch without merging.
func (p *IsolationPolicy) RollbackTask(ctx context.Context, runID string, render *Renderer) {
	if _, ok := p.active[runID]; !ok {
		return
	}
	p.removeWorktree(ctx, runID)
}

// Cleanup removes all active worktrees. Called from deferred cleanup.
func (p *IsolationPolicy) Cleanup() {
	for runID := range p.active {
		p.removeWorktree(context.Background(), runID)
	}
}

// PruneOrphans cleans up worktree metadata from previous crashes.
func (p *IsolationPolicy) PruneOrphans(ctx context.Context) error {
	if p.mode == IsolationOff {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", p.mainDir, "worktree", "prune").CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree prune: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (p *IsolationPolicy) removeWorktree(ctx context.Context, runID string) {
	info, ok := p.active[runID]
	if !ok {
		return
	}
	delete(p.active, runID)
	if ctx == nil {
		ctx = context.Background()
	}

	_ = exec.CommandContext(ctx, "git", "-C", p.mainDir, "worktree", "remove", "--force", info.path).Run()
	_ = exec.CommandContext(ctx, "git", "-C", p.mainDir, "branch", "-D", info.branch).Run()
}

// CaptureGitDiff runs git diff in the given directory and returns the output
// truncated to maxLines. Returns empty string on any error.
func CaptureGitDiff(workdir string, maxLines int) string {
	statCmd := exec.Command("git", "-C", workdir, "diff", "--stat")
	statOut, err := statCmd.Output()
	if err != nil {
		return ""
	}

	diffCmd := exec.Command("git", "-C", workdir, "diff")
	diffOut, err := diffCmd.Output()
	if err != nil {
		return strings.TrimSpace(string(statOut))
	}

	stat := strings.TrimSpace(string(statOut))
	diff := truncateOutput(strings.TrimSpace(string(diffOut)), maxLines)
	if stat == "" && diff == "" {
		return ""
	}

	var b strings.Builder
	if stat != "" {
		fmt.Fprintf(&b, "%s\n\n", stat)
	}
	if diff != "" {
		b.WriteString(diff)
	}
	return strings.TrimSpace(b.String())
}
