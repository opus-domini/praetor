package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store manages mutable runner state files and runtime artifacts.
type Store struct {
	Root string
}

// RunLock represents one acquired runtime lock owned by this process.
type RunLock struct {
	Path  string
	Token string
}

// NewStore builds a store with a validated root path.
func NewStore(root string) *Store {
	root = strings.TrimSpace(root)
	if root == "" {
		projectRoot, err := ProjectStateRootForDir(".")
		if err == nil {
			root = projectRoot
		} else {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr == nil {
				root = filepath.Join(homeDir, PraetorHomeDirName)
			} else {
				root = filepath.Join(".", PraetorHomeDirName)
			}
		}
	}
	return &Store{Root: root}
}

// Init ensures all required state directories exist.
func (s *Store) Init() error {
	dirs := []string{
		s.CheckpointsDir(),
		s.CostsDir(),
		s.FeedbackDir(),
		s.LocksDir(),
		s.LogsDir(),
		s.RetriesDir(),
		s.SnapshotsDir(),
		s.StateDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create state directory %s: %w", dir, err)
		}
	}
	return nil
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

func (s *Store) RetriesDir() string {
	return filepath.Join(s.Root, "retries")
}

func (s *Store) SnapshotsDir() string {
	return filepath.Join(s.Root, "snapshots")
}

func (s *Store) CostsDir() string {
	return filepath.Join(s.Root, "costs")
}

func (s *Store) StateDir() string {
	return filepath.Join(s.Root, "state")
}

// PlanBaseName returns a state-safe basename for one plan file.
func (s *Store) PlanBaseName(planFile string) string {
	return strings.TrimSuffix(filepath.Base(planFile), filepath.Ext(planFile))
}

// RuntimeKey returns the collision-resistant key used for all plan runtime artifacts.
func (s *Store) RuntimeKey(planFile string) string {
	clean := strings.TrimSpace(planFile)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	if real, err := filepath.EvalSymlinks(clean); err == nil {
		clean = real
	}

	baseName := strings.TrimSpace(filepath.Base(clean))
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	baseName = sanitizePathToken(baseName)
	if baseName == "" {
		baseName = "plan"
	}

	hash := sha256.Sum256([]byte(clean))
	return fmt.Sprintf("%s--%s", baseName, hex.EncodeToString(hash[:])[:12])
}

func (s *Store) legacyPlanBaseName(planFile string) string {
	return strings.TrimSuffix(filepath.Base(planFile), filepath.Ext(planFile))
}

func (s *Store) stateFileV2(planFile string) string {
	return filepath.Join(s.StateDir(), s.RuntimeKey(planFile)+".state.json")
}

func (s *Store) stateFileLegacy(planFile string) string {
	return filepath.Join(s.StateDir(), s.legacyPlanBaseName(planFile)+".state.json")
}

// StateFile returns the mutable state file path for a plan.
func (s *Store) StateFile(planFile string) string {
	return s.stateFileV2(planFile)
}

func (s *Store) lockFileV2(planFile string) string {
	return filepath.Join(s.LocksDir(), s.RuntimeKey(planFile)+".lock")
}

func (s *Store) lockFileLegacy(planFile string) string {
	return filepath.Join(s.LocksDir(), s.legacyPlanBaseName(planFile)+".lock")
}

// LockFile returns the lock file path for a plan.
func (s *Store) LockFile(planFile string) string {
	return s.lockFileV2(planFile)
}

func (s *Store) lockCandidates(planFile string) []string {
	v2 := s.lockFileV2(planFile)
	legacy := s.lockFileLegacy(planFile)
	if v2 == legacy {
		return []string{v2}
	}
	return []string{v2, legacy}
}

func (s *Store) currentCheckpointFile(planFile string) string {
	return filepath.Join(s.CheckpointsDir(), s.RuntimeKey(planFile)+".state")
}
