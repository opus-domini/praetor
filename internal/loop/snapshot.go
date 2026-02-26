package loop

import (
	"time"

	localstate "github.com/opus-domini/praetor/internal/state"
)

// LocalSnapshot — type alias delegating to state.LocalSnapshot.
type LocalSnapshot = localstate.LocalSnapshot

// LocalSnapshotEvent — type alias delegating to state.SnapshotEvent.
type LocalSnapshotEvent = localstate.SnapshotEvent

// LocalSnapshotStore — type alias delegating to state.LocalSnapshotStore.
type LocalSnapshotStore = localstate.LocalSnapshotStore

func NewLocalSnapshotStore(projectRoot, runID string) *LocalSnapshotStore {
	return localstate.NewLocalSnapshotStore(projectRoot, runID)
}

func LoadLatestLocalSnapshot(projectRoot, planFile string) (LocalSnapshot, string, error) {
	return localstate.LoadLatestLocalSnapshot(projectRoot, planFile)
}

func snapshotTimestamp(value string) time.Time {
	return localstate.ParseTimestamp(value)
}
