package loop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SaveGitSnapshot records the current HEAD commit for a run.
func (s *Store) SaveGitSnapshot(runID, workdir string) error {
	cmd := exec.Command("git", "-C", workdir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return errors.New("git rev-parse HEAD returned empty")
	}
	path := filepath.Join(s.SnapshotsDir(), runID+".sha")
	if err := os.WriteFile(path, []byte(sha+"\n"), 0o644); err != nil {
		return fmt.Errorf("write git snapshot: %w", err)
	}
	return nil
}

// GitWorktreeDirty reports whether tracked or untracked changes exist.
func (s *Store) GitWorktreeDirty(workdir string) (bool, error) {
	cmd := exec.Command("git", "-C", workdir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// RollbackGitSnapshot resets the working tree to the saved snapshot.
func (s *Store) RollbackGitSnapshot(runID, workdir string) error {
	path := filepath.Join(s.SnapshotsDir(), runID+".sha")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read git snapshot: %w", err)
	}
	sha := strings.TrimSpace(string(data))
	if sha == "" {
		return nil
	}

	resetCmd := exec.Command("git", "-C", workdir, "reset", "--hard", sha)
	if out, err := resetCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard %s: %w: %s", sha, err, strings.TrimSpace(string(out)))
	}
	cleanCmd := exec.Command("git", "-C", workdir, "clean", "-fd")
	if out, err := cleanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean -fd: %w: %s", err, strings.TrimSpace(string(out)))
	}

	_ = os.Remove(path)
	return nil
}

// DiscardGitSnapshot removes a snapshot file without rollback.
func (s *Store) DiscardGitSnapshot(runID string) error {
	path := filepath.Join(s.SnapshotsDir(), runID+".sha")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("discard git snapshot: %w", err)
	}
	return nil
}
