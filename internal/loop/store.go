package loop

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	lockSchemaVersion = 2
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

type lockMeta struct {
	Version   int    `json:"version"`
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
	StartedAt string `json:"started_at"`
	Token     string `json:"token"`
	Runtime   string `json:"runtime_key"`
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

func (s *Store) findStateFile(planFile string) (string, bool, error) {
	v2 := s.stateFileV2(planFile)
	if _, err := os.Stat(v2); err == nil {
		return v2, false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat state file: %w", err)
	}

	legacy := s.stateFileLegacy(planFile)
	if _, err := os.Stat(legacy); err == nil {
		return legacy, true, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat legacy state file: %w", err)
	}
	return v2, false, nil
}

func (s *Store) migrateStateFileIfNeeded(planFile, source string, state State) error {
	target := s.stateFileV2(planFile)
	if source == target {
		return nil
	}
	if err := writeJSONFile(target, state); err != nil {
		return fmt.Errorf("migrate legacy state file: %w", err)
	}
	_ = os.Remove(source)
	return nil
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

// LoadOrInitializeState creates state from plan or merges existing state after plan changes.
func (s *Store) LoadOrInitializeState(planFile string, plan Plan) (State, error) {
	if err := s.Init(); err != nil {
		return State{}, err
	}

	checksum, err := PlanChecksum(planFile)
	if err != nil {
		return State{}, err
	}

	stateFile, legacy, err := s.findStateFile(planFile)
	if err != nil {
		return State{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		state := State{
			PlanFile:     planFile,
			PlanChecksum: checksum,
			CreatedAt:    now,
			UpdatedAt:    now,
			Tasks:        stateTasksFromPlan(plan),
		}
		if err := writeJSONFile(s.StateFile(planFile), state); err != nil {
			return State{}, err
		}
		return state, nil
	} else if err != nil {
		return State{}, fmt.Errorf("stat state file: %w", err)
	}

	state, err := s.ReadState(planFile)
	if err != nil {
		return State{}, err
	}

	if state.PlanChecksum == checksum {
		if legacy {
			if migrateErr := s.migrateStateFileIfNeeded(planFile, stateFile, state); migrateErr != nil {
				return State{}, migrateErr
			}
		}
		return state, nil
	}

	merged := mergeState(planFile, checksum, state, plan)
	if err := writeJSONFile(s.StateFile(planFile), merged); err != nil {
		return State{}, err
	}
	if legacy {
		_ = os.Remove(stateFile)
	}
	return merged, nil
}

// ReadState reads state from disk.
func (s *Store) ReadState(planFile string) (State, error) {
	path, legacy, err := s.findStateFile(planFile)
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode state file: %w", err)
	}
	if legacy {
		if migrateErr := s.migrateStateFileIfNeeded(planFile, path, state); migrateErr != nil {
			return State{}, migrateErr
		}
	}
	return state, nil
}

// WriteState persists state atomically.
func (s *Store) WriteState(planFile string, state State) error {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeJSONFile(s.StateFile(planFile), state)
}

func stateTasksFromPlan(plan Plan) []StateTask {
	tasks := make([]StateTask, 0, len(plan.Tasks))
	for i, task := range plan.Tasks {
		tasks = append(tasks, StateTask{
			ID:          canonicalTaskID(task, i),
			Title:       strings.TrimSpace(task.Title),
			DependsOn:   normalizedDependsOn(task.DependsOn),
			Executor:    normalizeAgent(task.Executor),
			Reviewer:    normalizeAgent(task.Reviewer),
			Model:       strings.TrimSpace(task.Model),
			Description: strings.TrimSpace(task.Description),
			Criteria:    strings.TrimSpace(task.Criteria),
			Status:      TaskStatusOpen,
		})
	}
	return tasks
}

func normalizedDependsOn(dependsOn []string) []string {
	if len(dependsOn) == 0 {
		return nil
	}
	result := make([]string, 0, len(dependsOn))
	for _, dep := range dependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		result = append(result, dep)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeState(planFile, checksum string, previous State, plan Plan) State {
	statusByID := make(map[string]TaskStatus, len(previous.Tasks))
	for _, task := range previous.Tasks {
		statusByID[task.ID] = task.Status
	}

	now := time.Now().UTC().Format(time.RFC3339)
	merged := State{
		PlanFile:     planFile,
		PlanChecksum: checksum,
		CreatedAt:    previous.CreatedAt,
		UpdatedAt:    now,
		Tasks:        stateTasksFromPlan(plan),
	}
	if merged.CreatedAt == "" {
		merged.CreatedAt = now
	}

	for i, task := range merged.Tasks {
		if status, ok := statusByID[task.ID]; ok {
			merged.Tasks[i].Status = status
		}
	}
	return merged
}

func writeJSONFile(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json file %s: %w", path, err)
	}
	encoded = append(encoded, '\n')

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, encoded, 0o644); err != nil {
		return fmt.Errorf("write tmp file %s: %w", tmpFile, err)
	}
	if err := os.Rename(tmpFile, path); err != nil {
		return fmt.Errorf("rename tmp file %s: %w", path, err)
	}
	return nil
}

// TaskSignature returns a stable signature used by retries and feedback files.
func TaskSignature(taskKey string) string {
	hash := sha256.Sum256([]byte(taskKey))
	return hex.EncodeToString(hash[:])
}

// TaskKey builds the signature key for a task.
func TaskKey(index int, task StateTask) string {
	if strings.TrimSpace(task.ID) != "" {
		return "id:" + strings.TrimSpace(task.ID)
	}
	return fmt.Sprintf("index:%d:title:%s", index, strings.TrimSpace(task.Title))
}

// TaskSignatureForPlan returns the stable, plan-scoped signature for retries/feedback.
func (s *Store) TaskSignatureForPlan(planFile string, index int, task StateTask) string {
	scope := s.RuntimeKey(planFile) + "|" + TaskKey(index, task)
	return TaskSignature(scope)
}

// ReadRetryCount reads current retry count for one task signature.
func (s *Store) ReadRetryCount(signature string) (int, error) {
	path := filepath.Join(s.RetriesDir(), signature+".count")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read retry file: %w", err)
	}

	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse retry count for %s: %w", signature, err)
	}
	if value < 0 {
		return 0, nil
	}
	return value, nil
}

// IncrementRetryCount increments retry count and returns the new value.
func (s *Store) IncrementRetryCount(signature string) (int, error) {
	count, err := s.ReadRetryCount(signature)
	if err != nil {
		return 0, err
	}
	count++

	path := filepath.Join(s.RetriesDir(), signature+".count")
	if err := os.WriteFile(path, []byte(strconv.Itoa(count)), 0o644); err != nil {
		return 0, fmt.Errorf("write retry file: %w", err)
	}
	return count, nil
}

// ClearRetryCount deletes a retry counter file.
func (s *Store) ClearRetryCount(signature string) error {
	path := filepath.Join(s.RetriesDir(), signature+".count")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove retry file: %w", err)
	}
	return nil
}

// ReadFeedback reads previous reviewer feedback for a task signature.
func (s *Store) ReadFeedback(signature string) (string, error) {
	path := filepath.Join(s.FeedbackDir(), signature+".txt")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read feedback file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteFeedback persists reviewer feedback for a task signature.
func (s *Store) WriteFeedback(signature, feedback string) error {
	path := filepath.Join(s.FeedbackDir(), signature+".txt")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(feedback)), 0o644); err != nil {
		return fmt.Errorf("write feedback file: %w", err)
	}
	return nil
}

// ClearFeedback deletes a feedback file.
func (s *Store) ClearFeedback(signature string) error {
	path := filepath.Join(s.FeedbackDir(), signature+".txt")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove feedback file: %w", err)
	}
	return nil
}

// AcquireRunLock acquires a lock for one plan run.
func (s *Store) AcquireRunLock(planFile string, force bool) (RunLock, error) {
	if err := s.Init(); err != nil {
		return RunLock{}, err
	}

	runtimeKey := s.RuntimeKey(planFile)
	lockPath := s.LockFile(planFile)
	hostname, _ := os.Hostname()

	for _, legacyPath := range s.lockCandidates(planFile)[1:] {
		data, err := os.ReadFile(legacyPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return RunLock{}, fmt.Errorf("read lock file: %w", err)
		}
		meta := parseLockFile(data)
		if meta.PID > 0 && processIsRunning(meta.PID) && !force {
			return RunLock{}, fmt.Errorf("plan is already running (pid=%d, started=%s); use --force to override", meta.PID, meta.StartedAt)
		}
		if force || meta.PID <= 0 || !processIsRunning(meta.PID) {
			_ = os.Remove(legacyPath)
		}
	}

	for range 4 {
		token, err := randomHex(12)
		if err != nil {
			return RunLock{}, err
		}
		meta := lockMeta{
			Version:   lockSchemaVersion,
			PID:       os.Getpid(),
			Hostname:  strings.TrimSpace(hostname),
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			Token:     token,
			Runtime:   runtimeKey,
		}
		payload, err := json.Marshal(meta)
		if err != nil {
			return RunLock{}, fmt.Errorf("encode lock metadata: %w", err)
		}
		payload = append(payload, '\n')

		file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if _, writeErr := file.Write(payload); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return RunLock{}, fmt.Errorf("write lock file: %w", writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(lockPath)
				return RunLock{}, fmt.Errorf("close lock file: %w", closeErr)
			}
			return RunLock{Path: lockPath, Token: token}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return RunLock{}, fmt.Errorf("open lock file: %w", err)
		}

		data, readErr := os.ReadFile(lockPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return RunLock{}, fmt.Errorf("read lock file: %w", readErr)
		}
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}

		existing := parseLockFile(data)
		if existing.PID > 0 && processIsRunning(existing.PID) && !force {
			return RunLock{}, fmt.Errorf("plan is already running (pid=%d, started=%s); use --force to override", existing.PID, existing.StartedAt)
		}
		if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return RunLock{}, fmt.Errorf("remove stale lock file: %w", removeErr)
		}
	}
	return RunLock{}, errors.New("unable to acquire lock after multiple attempts")
}

// ReleaseRunLock releases a plan lock.
func (s *Store) ReleaseRunLock(lock RunLock) error {
	lockPath := strings.TrimSpace(lock.Path)
	if lockPath == "" {
		return nil
	}

	data, err := os.ReadFile(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}
	existing := parseLockFile(data)
	if existing.Token == "" {
		return errors.New("lock file does not contain ownership token")
	}
	if strings.TrimSpace(lock.Token) == "" || existing.Token != strings.TrimSpace(lock.Token) {
		return fmt.Errorf("lock ownership mismatch for %s", lockPath)
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove lock file: %w", err)
	}
	return nil
}

func parseLockFile(data []byte) lockMeta {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return lockMeta{}
	}

	var meta lockMeta
	if err := json.Unmarshal([]byte(trimmed), &meta); err == nil {
		return meta
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 {
		return lockMeta{}
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	started := ""
	if len(lines) > 1 {
		started = strings.TrimSpace(lines[1])
	}
	return lockMeta{
		PID:       pid,
		StartedAt: started,
	}
}

func processIsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

func randomHex(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("random size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// DetectStuckTasks reports open tasks that already reached retry limit.
func (s *Store) DetectStuckTasks(planFile string, state State, maxRetries int) ([]string, error) {
	if maxRetries <= 0 {
		return nil, nil
	}

	report := make([]string, 0)
	for idx, task := range state.Tasks {
		if task.Status != TaskStatusOpen {
			continue
		}
		signature := s.TaskSignatureForPlan(planFile, idx, task)
		retries, err := s.ReadRetryCount(signature)
		if err != nil {
			return nil, err
		}
		if retries >= maxRetries {
			report = append(report, fmt.Sprintf("%s: %s (%d/%d retries)", task.ID, task.Title, retries, maxRetries))
		}
	}
	return report, nil
}

// ResetPlanRuntime removes state, lock, retries and feedback for plan tasks.
func (s *Store) ResetPlanRuntime(planFile string, plan Plan) (int, error) {
	removed := 0
	for _, statePath := range []string{s.stateFileV2(planFile), s.stateFileLegacy(planFile)} {
		if err := os.Remove(statePath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove state file: %w", err)
		}
	}

	for _, lockPath := range s.lockCandidates(planFile) {
		if err := os.Remove(lockPath); err == nil {
			removed++
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("remove lock file: %w", err)
		}
	}

	stateTasks := stateTasksFromPlan(plan)
	for idx, task := range stateTasks {
		signature := s.TaskSignatureForPlan(planFile, idx, task)
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

// CostEntry records one agent invocation's metrics.
type CostEntry struct {
	Timestamp string
	RunID     string
	TaskID    string
	Agent     string
	Role      string
	DurationS float64
	Status    string
	CostUSD   float64
}

// WriteTaskMetrics appends one cost entry to the tracking ledger.
func (s *Store) WriteTaskMetrics(entry CostEntry) error {
	path := filepath.Join(s.CostsDir(), "tracking.tsv")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open cost tracking file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock cost tracking file: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat cost tracking file: %w", err)
	}
	if info.Size() == 0 {
		if _, err := fmt.Fprint(f, "timestamp\trun_id\ttask_id\tagent\trole\tduration_s\tstatus\tcost_usd\n"); err != nil {
			return fmt.Errorf("write cost header: %w", err)
		}
	}

	if _, err := fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\t%.2f\t%s\t%.6f\n",
		entry.Timestamp, entry.RunID, entry.TaskID, entry.Agent, entry.Role,
		entry.DurationS, entry.Status, entry.CostUSD); err != nil {
		return fmt.Errorf("write cost entry: %w", err)
	}
	return nil
}

// CheckpointEntry records one state transition in the audit log.
type CheckpointEntry struct {
	Timestamp string
	Status    string
	TaskID    string
	Signature string
	RunID     string
	Message   string
}

// WriteCheckpoint appends to the history log and overwrites the current checkpoint.
func (s *Store) WriteCheckpoint(planFile string, entry CheckpointEntry) error {
	historyPath := filepath.Join(s.CheckpointsDir(), "history.tsv")
	f, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open checkpoint history: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock checkpoint history: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat checkpoint history: %w", err)
	}
	if info.Size() == 0 {
		if _, err := fmt.Fprint(f, "timestamp\tstatus\ttask_id\tsignature\trun_id\tmessage\n"); err != nil {
			return fmt.Errorf("write checkpoint header: %w", err)
		}
	}

	if _, err := fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\t%s\n",
		entry.Timestamp, entry.Status, entry.TaskID, entry.Signature,
		entry.RunID, entry.Message); err != nil {
		return fmt.Errorf("write checkpoint entry: %w", err)
	}

	currentPath := s.currentCheckpointFile(planFile)
	content := fmt.Sprintf("timestamp=%s\nstatus=%s\ntask_id=%s\nsignature=%s\nrun_id=%s\nmessage=%s\n",
		entry.Timestamp, entry.Status, entry.TaskID, entry.Signature,
		entry.RunID, entry.Message)
	if err := os.WriteFile(currentPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write current checkpoint: %w", err)
	}
	return nil
}
