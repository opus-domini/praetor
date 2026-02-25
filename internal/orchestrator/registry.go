package orchestrator

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Registry stores available providers by ID.
type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderID]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{providers: map[ProviderID]Provider{}}
}

// Register inserts a provider into the registry.
func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}

	id := provider.ID()
	if id == "" {
		return errors.New("provider ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = map[ProviderID]Provider{}
	}
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("provider already registered: %s", id)
	}
	r.providers[id] = provider
	return nil
}

// Get returns a provider by ID.
func (r *Registry) Get(id ProviderID) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[id]
	return provider, ok
}

// IDs returns sorted provider IDs.
func (r *Registry) IDs() []ProviderID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]ProviderID, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
