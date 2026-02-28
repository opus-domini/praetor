package state

import (
	"crypto/sha256"
	"encoding/hex"
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
	Version           int             `json:"version"`
	RunID             string          `json:"run_id"`
	PlanSlug          string          `json:"plan_slug"`
	PlanChecksum      string          `json:"plan_checksum"`
	ProjectRoot       string          `json:"project_root"`
	ManifestPath      string          `json:"manifest_path,omitempty"`
	ManifestHash      string          `json:"manifest_hash,omitempty"`
	ManifestTruncated bool            `json:"manifest_truncated,omitempty"`
	Phase             string          `json:"phase"`
	Message           string          `json:"message"`
	Outcome           string          `json:"outcome,omitempty"`
	Iteration         int             `json:"iteration"`
	Timestamp         string          `json:"timestamp"`
	State             json.RawMessage `json:"state"`
}

// SnapshotEvent is one append-only event in events.log.
type SnapshotEvent struct {
	Timestamp string `json:"timestamp"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	TaskID    string `json:"task_id,omitempty"`
	Message   string `json:"message"`
}

// SnapshotStore manages local project snapshots under <runtimeRoot>/<run-id>.
type SnapshotStore struct {
	runtimeRoot string
	runID       string
	rootDir     string
}

func NewSnapshotStore(runtimeRoot, runID string) *SnapshotStore {
	rootDir := filepath.Join(runtimeRoot, runID)
	return &SnapshotStore{
		runtimeRoot: strings.TrimSpace(runtimeRoot),
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

func (s *SnapshotStore) Init(planSlug, planChecksum string) error {
	if strings.TrimSpace(s.rootDir) == "" {
		return errors.New("local snapshot root is required")
	}
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return fmt.Errorf("create local snapshot root: %w", err)
	}
	meta := map[string]string{
		"run_id":        s.runID,
		"runtime_root":  s.runtimeRoot,
		"plan_slug":     strings.TrimSpace(planSlug),
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
		snapshot.ProjectRoot = s.runtimeRoot
	}
	if strings.TrimSpace(snapshot.Timestamp) == "" {
		snapshot.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if len(snapshot.State) == 0 {
		snapshot.State = json.RawMessage("{}")
	}
	encoded, err := marshalJSON(snapshot)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err := writeBytesAtomic(s.snapshotPath(), encoded); err != nil {
		return err
	}
	checksum := sha256Hex(encoded)
	return s.updateMeta(map[string]string{
		"snapshot_sha256":     checksum,
		"snapshot_updated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *SnapshotStore) updateMeta(fields map[string]string) error {
	meta := map[string]string{}
	if data, err := os.ReadFile(s.metaPath()); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	for key, value := range fields {
		meta[key] = value
	}
	return writeJSONAtomic(s.metaPath(), meta)
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

func LoadLatestSnapshot(runtimeRoot, planSlug string) (Snapshot, string, error) {
	entries, err := os.ReadDir(runtimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, "", nil
	}
	if err != nil {
		return Snapshot{}, "", fmt.Errorf("read local runtime root: %w", err)
	}

	planSlug = strings.TrimSpace(planSlug)
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
		if !snapshotChecksumMatches(filepath.Join(runtimeRoot, entry.Name(), "meta.json"), data) {
			continue
		}
		candidate := Snapshot{}
		if unmarshalErr := json.Unmarshal(data, &candidate); unmarshalErr != nil {
			continue
		}
		if strings.TrimSpace(candidate.PlanSlug) != planSlug {
			continue
		}
		if latestPath == "" || ParseTimestamp(candidate.Timestamp).After(ParseTimestamp(latest.Timestamp)) {
			latest = candidate
			latestPath = path
		}
	}
	return latest, latestPath, nil
}

type runSnapshotMeta struct {
	dir       string
	timestamp time.Time
}

// PruneLocalSnapshots keeps only the most recent keepLast run directories.
// When keepLast <= 0, no pruning is performed.
func PruneLocalSnapshots(runtimeRoot string, keepLast int) error {
	if keepLast <= 0 {
		return nil
	}
	entries, err := os.ReadDir(runtimeRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local runtime root: %w", err)
	}

	runs := make([]runSnapshotMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(runtimeRoot, entry.Name())
		ts := time.Time{}

		if data, readErr := os.ReadFile(filepath.Join(runDir, "snapshot.json")); readErr == nil {
			s := Snapshot{}
			if jsonErr := json.Unmarshal(data, &s); jsonErr == nil {
				ts = ParseTimestamp(s.Timestamp)
			}
		}
		if ts.IsZero() {
			if info, statErr := os.Stat(runDir); statErr == nil {
				ts = info.ModTime().UTC()
			}
		}
		runs = append(runs, runSnapshotMeta{dir: runDir, timestamp: ts})
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].timestamp.After(runs[j].timestamp)
	})

	for idx, run := range runs {
		if idx < keepLast {
			continue
		}
		if err := os.RemoveAll(run.dir); err != nil {
			return fmt.Errorf("remove old local runtime %s: %w", run.dir, err)
		}
	}
	return nil
}

func snapshotChecksumMatches(metaPath string, snapshotData []byte) bool {
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		// Metadata missing is tolerated (checksum validation is best-effort).
		return true
	}
	meta := map[string]any{}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return true
	}
	expected := strings.TrimSpace(anyToString(meta["snapshot_sha256"]))
	if expected == "" {
		return true
	}
	return strings.EqualFold(expected, sha256Hex(snapshotData))
}

func anyToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
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

func writeJSONAtomic(path string, value any) error {
	data, err := marshalJSON(value)
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, data)
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	data = append(data, '\n')
	return data, nil
}

func writeBytesAtomic(path string, data []byte) error {
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

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
