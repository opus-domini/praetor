package agent

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Registry stores available cognitive agents by ID.
type Registry struct {
	mu     sync.RWMutex
	agents map[ID]Agent
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry {
	return &Registry{agents: map[ID]Agent{}}
}

// Register inserts one agent into the registry.
func (r *Registry) Register(agent Agent) error {
	if agent == nil {
		return errors.New("agent cannot be nil")
	}
	id := Normalize(string(agent.ID()))
	if id == "" {
		return errors.New("agent id cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.agents == nil {
		r.agents = map[ID]Agent{}
	}
	if _, exists := r.agents[id]; exists {
		return fmt.Errorf("agent already registered: %s", id)
	}
	r.agents[id] = agent
	return nil
}

// Get returns one agent by ID.
func (r *Registry) Get(id ID) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[Normalize(string(id))]
	return agent, ok
}

// IDs returns sorted registered IDs.
func (r *Registry) IDs() []ID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]ID, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
