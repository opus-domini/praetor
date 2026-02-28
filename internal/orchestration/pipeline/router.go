package pipeline

import (
	"fmt"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/domain"
)

// resolveExecutorWithRouting enhances resolveExecutor with intelligent selection
// from available agents when the default executor is not reachable.
func resolveExecutorWithRouting(defaultExecutor domain.Agent, available []agent.ID) (domain.Agent, error) {
	// 1. Default executor — check if it's in the available set
	if defaultExecutor != "" && defaultExecutor != domain.AgentNone {
		defID := agent.Normalize(string(defaultExecutor))
		if isAvailable(defID, available) {
			return defaultExecutor, nil
		}
	}

	// 2. Auto-select from available agents
	if len(available) == 0 {
		// No availability data — fall back to resolveExecutor behavior
		return resolveExecutor(defaultExecutor)
	}

	// Prefer CLI agents over REST (CLI agents tend to have richer code interaction)
	var bestCLI, bestREST agent.ID
	for _, id := range available {
		entry, ok := agent.LookupCatalog(id)
		if !ok {
			continue
		}
		if !entry.Capabilities.SupportsExecute {
			continue
		}
		switch entry.Transport {
		case agent.TransportCLI:
			if bestCLI == "" {
				bestCLI = id
			}
		case agent.TransportREST:
			if bestREST == "" {
				bestREST = id
			}
		}
	}

	if bestCLI != "" {
		return domain.Agent(bestCLI), nil
	}
	if bestREST != "" {
		return domain.Agent(bestREST), nil
	}

	return "", fmt.Errorf("no available executor agent found (default %q unavailable)", defaultExecutor)
}

func isAvailable(target agent.ID, available []agent.ID) bool {
	for _, id := range available {
		if id == target {
			return true
		}
	}
	return false
}
