package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
)

// CounterKey identifies a unique (agent, role, status) triple.
type CounterKey struct {
	Agent  string
	Role   string
	Status string
}

// Counters tracks thread-safe invocation metrics.
type Counters struct {
	mu         sync.Mutex
	Counts     map[CounterKey]int
	TotalCost  float64
	TotalCalls int
}

// NewCounters creates an initialized Counters.
func NewCounters() *Counters {
	return &Counters{
		Counts: make(map[CounterKey]int),
	}
}

// Snapshot returns a copy of the current counter state.
func (c *Counters) Snapshot() (counts map[CounterKey]int, totalCost float64, totalCalls int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	counts = make(map[CounterKey]int, len(c.Counts))
	for k, v := range c.Counts {
		counts[k] = v
	}
	return counts, c.TotalCost, c.TotalCalls
}

func (c *Counters) record(agent, role, status string, cost float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := CounterKey{Agent: agent, Role: role, Status: status}
	c.Counts[key]++
	c.TotalCost += cost
	c.TotalCalls++
}

// Metrics creates a middleware that records invocation counts and costs.
func Metrics(counters *Counters) Middleware {
	return func(next domain.AgentRuntime) domain.AgentRuntime {
		return runtimeFunc(func(ctx context.Context, req domain.AgentRequest) (domain.AgentResult, error) {
			start := time.Now()
			result, err := next.Run(ctx, req)
			_ = time.Since(start)

			status := "ok"
			if err != nil {
				status = "error"
			}
			counters.record(string(req.Agent), req.Role, status, result.CostUSD)
			return result, err
		})
	}
}
