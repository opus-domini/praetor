package state

import (
	"encoding/json"
	"fmt"

	"github.com/opus-domini/praetor/internal/domain"
)

// LocalSnapshot captures transactional runtime state for one run.
type LocalSnapshot struct {
	Version           int          `json:"version"`
	RunID             string       `json:"run_id"`
	PlanSlug          string       `json:"plan_slug"`
	PlanChecksum      string       `json:"plan_checksum"`
	ProjectRoot       string       `json:"project_root"`
	ManifestPath      string       `json:"manifest_path,omitempty"`
	ManifestHash      string       `json:"manifest_hash,omitempty"`
	ManifestTruncated bool         `json:"manifest_truncated,omitempty"`
	Phase             string       `json:"phase"`
	Message           string       `json:"message"`
	Iteration         int          `json:"iteration"`
	Timestamp         string       `json:"timestamp"`
	State             domain.State `json:"state"`
}

// LocalSnapshotStore manages local project snapshots under <runtimeRoot>/<run-id>.
type LocalSnapshotStore struct {
	inner *SnapshotStore
}

func NewLocalSnapshotStore(runtimeRoot, runID string) *LocalSnapshotStore {
	return &LocalSnapshotStore{inner: NewSnapshotStore(runtimeRoot, runID)}
}

func (s *LocalSnapshotStore) RootDir() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.RootDir()
}

func (s *LocalSnapshotStore) Init(planSlug, planChecksum string) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Init(planSlug, planChecksum)
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
	return s.inner.Save(Snapshot{
		Version:           snapshot.Version,
		RunID:             snapshot.RunID,
		PlanSlug:          snapshot.PlanSlug,
		PlanChecksum:      snapshot.PlanChecksum,
		ProjectRoot:       snapshot.ProjectRoot,
		ManifestPath:      snapshot.ManifestPath,
		ManifestHash:      snapshot.ManifestHash,
		ManifestTruncated: snapshot.ManifestTruncated,
		Phase:             snapshot.Phase,
		Message:           snapshot.Message,
		Iteration:         snapshot.Iteration,
		Timestamp:         snapshot.Timestamp,
		State:             statePayload,
	})
}

func (s *LocalSnapshotStore) AppendEvent(event SnapshotEvent) error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.AppendEvent(event)
}

func LoadLatestLocalSnapshot(runtimeRoot, planSlug string) (LocalSnapshot, string, error) {
	snapshot, path, err := LoadLatestSnapshot(runtimeRoot, planSlug)
	if err != nil {
		return LocalSnapshot{}, "", err
	}
	if path == "" {
		return LocalSnapshot{}, "", nil
	}
	state := domain.State{}
	if len(snapshot.State) > 0 {
		if err := json.Unmarshal(snapshot.State, &state); err != nil {
			return LocalSnapshot{}, "", fmt.Errorf("decode local snapshot state: %w", err)
		}
	}
	return LocalSnapshot{
		Version:           snapshot.Version,
		RunID:             snapshot.RunID,
		PlanSlug:          snapshot.PlanSlug,
		PlanChecksum:      snapshot.PlanChecksum,
		ProjectRoot:       snapshot.ProjectRoot,
		ManifestPath:      snapshot.ManifestPath,
		ManifestHash:      snapshot.ManifestHash,
		ManifestTruncated: snapshot.ManifestTruncated,
		Phase:             snapshot.Phase,
		Message:           snapshot.Message,
		Iteration:         snapshot.Iteration,
		Timestamp:         snapshot.Timestamp,
		State:             state,
	}, path, nil
}
