package loop

import localstate "github.com/opus-domini/praetor/internal/state"

// Store — type alias delegating to state.Store.
type Store = localstate.Store

// RunLock — type alias delegating to state.RunLock.
type RunLock = localstate.RunLock

// NewStore builds a store with validated root paths.
func NewStore(stateRoot, cacheRoot string) *Store {
	return localstate.NewStore(stateRoot, cacheRoot)
}
