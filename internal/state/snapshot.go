package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const snapshotSchemaVersion = 1

// Snapshot captures transactional runtime state for one run.
type Snapshot struct {
	Version      int             `json:"version"`
	RunID        string          `json:"run_id"`
	PlanFile     string          `json:"plan_file"`
	PlanChecksum string          `json:"plan_checksum"`
	ProjectRoot  string          `json:"project_root"`
	Phase        string          `json:"phase"`
	Message      string          `json:"message"`
	Iteration    int             `json:"iteration"`
	Timestamp    string          `json:"timestamp"`
	State        json.RawMessage `json:"state"`
}

// SnapshotEvent is one append-only event in events.log.
type SnapshotEvent struct {
	Timestamp string `json:"timestamp"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	TaskID    string `json:"task_id,omitempty"`
	Message   string `json:"message"`
}

// SnapshotStore manages local project snapshots under .praetor/runtime/<run-id>.
type SnapshotStore struct {
	projectRoot string
	runID       string
	rootDir     string
}

func NewSnapshotStore(projectRoot, runID string) *SnapshotStore {
	rootDir := filepath.Join(projectRoot, ".praetor", "runtime", runID)
	return &SnapshotStore{
		projectRoot: strings.TrimSpace(projectRoot),
		runID:       strings.TrimSpace(runID),
		rootDir:     rootDir,
	}
}

func (s *SnapshotStore) RootDir() string {
	return s.rootDir
}

func (s *SnapshotStore) snapshotPath() string {
	return filepath.Join(s.rootDir, "snapshot.json")
}

func (s *SnapshotStore) eventsPath() string {
	return filepath.Join(s.rootDir, "events.log")
}

func (s *SnapshotStore) lockPath() string {
	return filepath.Join(s.rootDir, "lock.json")
}

func (s *SnapshotStore) metaPath() string {
	return filepath.Join(s.rootDir, "meta.json")
}

func (s *SnapshotStore) Init(planFile, planChecksum string) error {
	if strings.TrimSpace(s.rootDir) == "" {
		return errors.New("local snapshot root is required")
	}
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return fmt.Errorf("create local snapshot root: %w", err)
	}
	meta := map[string]string{
		"run_id":        s.runID,
		"project_root":  s.projectRoot,
		"plan_file":     strings.TrimSpace(planFile),
		"plan_checksum": strings.TrimSpace(planChecksum),
		"created_at":    time.Now().UTC().Format(time.RFC3339),
	}
	return writeJSONAtomic(s.metaPath(), meta)
}

func (s *SnapshotStore) WriteLock(token string, pid int) error {
	payload := map[string]any{
		"run_id":     strings.TrimSpace(s.runID),
		"pid":        pid,
		"token":      strings.TrimSpace(token),
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	}
	return writeJSONAtomic(s.lockPath(), payload)
}

func (s *SnapshotStore) Save(snapshot Snapshot) error {
	snapshot.Version = snapshotSchemaVersion
	if strings.TrimSpace(snapshot.RunID) == "" {
		snapshot.RunID = s.runID
	}
	if strings.TrimSpace(snapshot.ProjectRoot) == "" {
		snapshot.ProjectRoot = s.projectRoot
	}
	if strings.TrimSpace(snapshot.Timestamp) == "" {
		snapshot.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if len(snapshot.State) == 0 {
		snapshot.State = json.RawMessage("{}")
	}
	return writeJSONAtomic(s.snapshotPath(), snapshot)
}

func (s *SnapshotStore) AppendEvent(event SnapshotEvent) error {
	if strings.TrimSpace(event.Timestamp) == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(event.RunID) == "" {
		event.RunID = s.runID
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode snapshot event: %w", err)
	}
	encoded = append(encoded, '\n')

	f, err := os.OpenFile(s.eventsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open snapshot event log: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(encoded); err != nil {
		return fmt.Errorf("write snapshot event: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync snapshot event log: %w", err)
	}
	return nil
}

func LoadLatestSnapshot(projectRoot, planFile string) (Snapshot, string, error) {
	runtimeRoot := filepath.Join(projectRoot, ".praetor", "runtime")
	entries, err := os.ReadDir(runtimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, "", nil
	}
	if err != nil {
		return Snapshot{}, "", fmt.Errorf("read local runtime root: %w", err)
	}

	planFile = strings.TrimSpace(planFile)
	latest := Snapshot{}
	latestPath := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(runtimeRoot, entry.Name(), "snapshot.json")
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		candidate := Snapshot{}
		if unmarshalErr := json.Unmarshal(data, &candidate); unmarshalErr != nil {
			continue
		}
		if strings.TrimSpace(candidate.PlanFile) != planFile {
			continue
		}
		if latestPath == "" || ParseTimestamp(candidate.Timestamp).After(ParseTimestamp(latest.Timestamp)) {
			latest = candidate
			latestPath = path
		}
	}
	return latest, latestPath, nil
}

func ParseTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func listSnapshots(projectRoot string) ([]string, error) {
	runtimeRoot := filepath.Join(projectRoot, ".praetor", "runtime")
	entries, err := os.ReadDir(runtimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read local runtime root: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(runtimeRoot, entry.Name(), "snapshot.json"))
	}
	sort.Strings(paths)
	return paths, nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json %s: %w", path, err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open tmp snapshot file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write tmp snapshot file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync tmp snapshot file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close tmp snapshot file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename snapshot file: %w", err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync snapshot directory: %w", err)
	}
	return nil
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
