package loop

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Store manages mutable runner state files and runtime artifacts.
type Store struct {
	Root string
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
		s.FeedbackDir(),
		s.LocksDir(),
		s.LogsDir(),
		s.RetriesDir(),
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

func (s *Store) StateDir() string {
	return filepath.Join(s.Root, "state")
}

// PlanBaseName returns a state-safe basename for one plan file.
func (s *Store) PlanBaseName(planFile string) string {
	return strings.TrimSuffix(filepath.Base(planFile), filepath.Ext(planFile))
}

// StateFile returns the mutable state file path for a plan.
func (s *Store) StateFile(planFile string) string {
	return filepath.Join(s.StateDir(), s.PlanBaseName(planFile)+".state.json")
}

// LockFile returns the lock file path for a plan.
func (s *Store) LockFile(planFile string) string {
	return filepath.Join(s.LocksDir(), s.PlanBaseName(planFile)+".lock")
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

	stateFile := s.StateFile(planFile)
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := os.Stat(stateFile); errors.Is(err, os.ErrNotExist) {
		state := State{
			PlanFile:     planFile,
			PlanChecksum: checksum,
			CreatedAt:    now,
			UpdatedAt:    now,
			Tasks:        stateTasksFromPlan(plan),
		}
		if err := writeJSONFile(stateFile, state); err != nil {
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
		return state, nil
	}

	merged := mergeState(planFile, checksum, state, plan)
	if err := writeJSONFile(stateFile, merged); err != nil {
		return State{}, err
	}
	return merged, nil
}

// ReadState reads state from disk.
func (s *Store) ReadState(planFile string) (State, error) {
	path := s.StateFile(planFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode state file: %w", err)
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
	hash := sha1.Sum([]byte(taskKey))
	return hex.EncodeToString(hash[:])
}

// TaskKey builds the signature key for a task.
func TaskKey(index int, task StateTask) string {
	if strings.TrimSpace(task.ID) != "" {
		return "id:" + strings.TrimSpace(task.ID)
	}
	return fmt.Sprintf("index:%d:title:%s", index, strings.TrimSpace(task.Title))
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
func (s *Store) AcquireRunLock(planFile string, force bool) (string, error) {
	if err := s.Init(); err != nil {
		return "", err
	}

	lockPath := s.LockFile(planFile)
	if data, err := os.ReadFile(lockPath); err == nil {
		pid, started := parseLockFile(data)
		if pid > 0 && processIsRunning(pid) && !force {
			return "", fmt.Errorf("plan is already running (pid=%d, started=%s); use --force to override", pid, started)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read lock file: %w", err)
	}

	content := fmt.Sprintf("%d\n%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(lockPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write lock file: %w", err)
	}
	return lockPath, nil
}

// ReleaseRunLock releases a plan lock.
func (s *Store) ReleaseRunLock(lockPath string) {
	if strings.TrimSpace(lockPath) == "" {
		return
	}
	_ = os.Remove(lockPath)
}

func parseLockFile(data []byte) (int, string) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		return 0, ""
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	started := ""
	if len(lines) > 1 {
		started = strings.TrimSpace(lines[1])
	}
	return pid, started
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

// DetectStuckTasks reports open tasks that already reached retry limit.
func (s *Store) DetectStuckTasks(state State, maxRetries int) ([]string, error) {
	if maxRetries <= 0 {
		return nil, nil
	}

	report := make([]string, 0)
	for idx, task := range state.Tasks {
		if task.Status != TaskStatusOpen {
			continue
		}
		signature := TaskSignature(TaskKey(idx, task))
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
	statePath := s.StateFile(planFile)
	if err := os.Remove(statePath); err == nil {
		removed++
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return removed, fmt.Errorf("remove state file: %w", err)
	}

	lockPath := s.LockFile(planFile)
	if err := os.Remove(lockPath); err == nil {
		removed++
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return removed, fmt.Errorf("remove lock file: %w", err)
	}

	stateTasks := stateTasksFromPlan(plan)
	for idx, task := range stateTasks {
		signature := TaskSignature(TaskKey(idx, task))
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
