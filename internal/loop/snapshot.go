package loop

import (
	"encoding/json"
	"fmt"
	"time"

	localstate "github.com/opus-domini/praetor/internal/state"
)

// LocalSnapshot captures transactional runtime state for one run.
type LocalSnapshot struct {
	Version      int    `json:"version"`
	RunID        string `json:"run_id"`
	PlanFile     string `json:"plan_file"`
	PlanChecksum string `json:"plan_checksum"`
	ProjectRoot  string `json:"project_root"`
	Phase        string `json:"phase"`
	Message      string `json:"message"`
	Iteration    int    `json:"iteration"`
	Timestamp    string `json:"timestamp"`
	State        State  `json:"state"`
}

// LocalSnapshotEvent is one append-only event in events.log.
type LocalSnapshotEvent = localstate.SnapshotEvent

// LocalSnapshotStore manages local project snapshots under .praetor/runtime/<run-id>.
type LocalSnapshotStore struct {
	inner *localstate.SnapshotStore
}

func NewLocalSnapshotStore(projectRoot, runID string) *LocalSnapshotStore {
	return &LocalSnapshotStore{inner: localstate.NewSnapshotStore(projectRoot, runID)}
}

func (s *LocalSnapshotStore) RootDir() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.RootDir()
}

func (s *LocalSnapshotStore) Init(planFile, planChecksum string) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Init(planFile, planChecksum)
}

func (s *LocalSnapshotStore) WriteLock(token string, pid int) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.WriteLock(token, pid)
}

func (s *LocalSnapshotStore) Save(snapshot LocalSnapshot) error {
	if s == nil || s.inner == nil {
		return nil
	}
	statePayload, err := json.Marshal(snapshot.State)
	if err != nil {
		return fmt.Errorf("encode snapshot state: %w", err)
	}
	return s.inner.Save(localstate.Snapshot{
		Version:      snapshot.Version,
		RunID:        snapshot.RunID,
		PlanFile:     snapshot.PlanFile,
		PlanChecksum: snapshot.PlanChecksum,
		ProjectRoot:  snapshot.ProjectRoot,
		Phase:        snapshot.Phase,
		Message:      snapshot.Message,
		Iteration:    snapshot.Iteration,
		Timestamp:    snapshot.Timestamp,
		State:        statePayload,
	})
}

func (s *LocalSnapshotStore) AppendEvent(event LocalSnapshotEvent) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.AppendEvent(event)
}

func LoadLatestLocalSnapshot(projectRoot, planFile string) (LocalSnapshot, string, error) {
	snapshot, path, err := localstate.LoadLatestSnapshot(projectRoot, planFile)
	if err != nil {
		return LocalSnapshot{}, "", err
	}
	if path == "" {
		return LocalSnapshot{}, "", nil
	}
	state := State{}
	if len(snapshot.State) > 0 {
		if err := json.Unmarshal(snapshot.State, &state); err != nil {
			return LocalSnapshot{}, "", fmt.Errorf("decode local snapshot state: %w", err)
		}
	}
	return LocalSnapshot{
		Version:      snapshot.Version,
		RunID:        snapshot.RunID,
		PlanFile:     snapshot.PlanFile,
		PlanChecksum: snapshot.PlanChecksum,
		ProjectRoot:  snapshot.ProjectRoot,
		Phase:        snapshot.Phase,
		Message:      snapshot.Message,
		Iteration:    snapshot.Iteration,
		Timestamp:    snapshot.Timestamp,
		State:        state,
	}, path, nil
}

func snapshotTimestamp(value string) time.Time {
	return localstate.ParseTimestamp(value)
}
