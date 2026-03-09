package mcp

import (
	"context"
	"time"

	"github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/config"
)

func registerExecTools(s *Server) {
	s.tools.register("doctor", "Check availability of all AI agent providers, respecting config overrides for binary paths and REST endpoints",
		objectSchema(map[string]any{}, nil),
		func(args map[string]any) ([]contentBlock, error) {
			cfg, _ := config.Load(s.projectDir)
			binaryOverrides := buildBinaryOverrides(cfg)
			restEndpoints := buildRESTEndpoints(cfg)

			prober := agent.NewProber(agent.WithTimeout(3 * time.Second))
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return jsonContent(prober.ProbeAll(ctx, binaryOverrides, restEndpoints))
		},
	)
}

func buildBinaryOverrides(cfg config.Config) map[agent.ID]string {
	overrides := make(map[agent.ID]string)
	if cfg.CodexBin != "" {
		overrides[agent.Codex] = cfg.CodexBin
	}
	if cfg.ClaudeBin != "" {
		overrides[agent.Claude] = cfg.ClaudeBin
	}
	if cfg.CopilotBin != "" {
		overrides[agent.Copilot] = cfg.CopilotBin
	}
	if cfg.GeminiBin != "" {
		overrides[agent.Gemini] = cfg.GeminiBin
	}
	if cfg.KimiBin != "" {
		overrides[agent.Kimi] = cfg.KimiBin
	}
	if cfg.OpenCodeBin != "" {
		overrides[agent.OpenCode] = cfg.OpenCodeBin
	}
	return overrides
}

func buildRESTEndpoints(cfg config.Config) map[agent.ID]string {
	endpoints := make(map[agent.ID]string)
	if cfg.OpenRouterURL != "" {
		endpoints[agent.OpenRouter] = cfg.OpenRouterURL
	}
	if cfg.OllamaURL != "" {
		endpoints[agent.Ollama] = cfg.OllamaURL
	}
	if cfg.LMStudioURL != "" {
		endpoints[agent.LMStudio] = cfg.LMStudioURL
	}
	return endpoints
}
