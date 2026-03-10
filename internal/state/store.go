package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// Store manages mutable runner state files and runtime artifacts.
// Root is the single per-project home directory holding all state, logs, plans, etc.
type Store struct {
	Root string
}

// RunLock represents one acquired runtime lock owned by this process.
type RunLock struct {
	Path  string
	Token string
}

// TaskLock represents one acquired task-level runtime lock.
type TaskLock struct {
	Path  string
	Token string
}

// NewStore builds a store with a validated root path.
func NewStore(root string) *Store {
	root = strings.TrimSpace(root)
	if root == "" {
		resolved, err := ProjectHomeForDir(".")
		if err == nil {
			root = resolved
		} else {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr == nil {
				root = filepath.Join(homeDir, ".praetor", "projects", "default")
			} else {
				root = filepath.Join(".", ".praetor")
			}
		}
	}
	return &Store{Root: root}
}

// Init ensures all required state directories exist.
func (s *Store) Init() error {
	dirs := []string{
		s.BriefsDir(),
		s.CheckpointsDir(),
		s.CostsDir(),
		s.FeedbackDir(),
		s.LocksDir(),
		s.LogsDir(),
		s.PlansDir(),
		s.RetriesDir(),
		s.RuntimeDir(),
		s.StateDir(),
		s.WorktreesDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create state directory %s: %w", dir, err)
		}
	}
	return nil
}

func (s *Store) BriefsDir() string {
	return filepath.Join(s.Root, "briefs")
}

// BriefFile returns the full path for a brief filename.
func (s *Store) BriefFile(filename string) string {
	return filepath.Join(s.BriefsDir(), filename)
}

func (s *Store) CheckpointsDir() string {
	return filepath.Join(s.Root, "checkpoints")
}

func (s *Store) FeedbackDir() string {
	return filepath.Join(s.Root, "feedback")
}

func (s *Store) LocksDir() string {
	return filepath.Join(s.Root, "locks")
}

func (s *Store) LogsDir() string {
	return filepath.Join(s.Root, "logs")
}

func (s *Store) PlansDir() string {
	return filepath.Join(s.Root, "plans")
}

func (s *Store) RetriesDir() string {
	return filepath.Join(s.Root, "retries")
}

func (s *Store) CostsDir() string {
	return filepath.Join(s.Root, "costs")
}

func (s *Store) StateDir() string {
	return filepath.Join(s.Root, "state")
}

func (s *Store) RuntimeDir() string {
	return filepath.Join(s.Root, "runtime")
}

func (s *Store) WorktreesDir() string {
	return filepath.Join(s.Root, "worktrees")
}

// PlanFile returns the full path for a plan slug.
func (s *Store) PlanFile(slug string) string {
	return filepath.Join(s.PlansDir(), slug+".json")
}

// RuntimeKey returns the key used for plan runtime artifacts.
// With slug-based identity, the slug itself is the key.
func (s *Store) RuntimeKey(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "plan"
	}
	return domain.SanitizePathToken(slug)
}

// StateFile returns the mutable state file path for a plan slug.
func (s *Store) StateFile(slug string) string {
	return filepath.Join(s.StateDir(), s.RuntimeKey(slug)+".state.json")
}

// LockFile returns the lock file path for a plan slug.
func (s *Store) LockFile(slug string) string {
	return filepath.Join(s.LocksDir(), s.RuntimeKey(slug)+".lock")
}

// TaskLockFile returns the lock file path for one task within a plan slug.
func (s *Store) TaskLockFile(slug, taskID string) string {
	name := s.RuntimeKey(slug) + "--" + domain.SanitizePathToken(taskID) + ".task.lock"
	return filepath.Join(s.LocksDir(), name)
}

func (s *Store) currentCheckpointFile(slug string) string {
	return filepath.Join(s.CheckpointsDir(), s.RuntimeKey(slug)+".state")
}
