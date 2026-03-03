package mcp

import (
	"context"
	"time"

	"github.com/opus-domini/praetor/internal/agent"
)

func registerExecTools(s *Server) {
	s.tools.register("doctor", "Check availability of all AI agent providers",
		objectSchema(map[string]any{}, nil),
		func(args map[string]any) ([]contentBlock, error) {
			prober := agent.NewProber(agent.WithTimeout(3 * time.Second))
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			type agentStatus struct {
				Agent     string `json:"agent"`
				Available bool   `json:"available"`
				Version   string `json:"version,omitempty"`
				Transport string `json:"transport"`
				Error     string `json:"error,omitempty"`
			}

			// Use ProbeAll with empty overrides to check all agents.
			results := prober.ProbeAll(ctx, nil, nil)
			statuses := make([]agentStatus, 0, len(results))
			for _, result := range results {
				status := agentStatus{
					Agent:     string(result.ID),
					Available: result.Healthy(),
					Version:   result.Version,
					Transport: string(result.Transport),
				}
				if !result.Healthy() && result.Detail != "" {
					status.Error = result.Detail
				}
				statuses = append(statuses, status)
			}
			return jsonContent(statuses)
		},
	)
}
