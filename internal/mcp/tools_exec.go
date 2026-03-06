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
			return jsonContent(prober.ProbeAll(ctx, nil, nil))
		},
	)
}
