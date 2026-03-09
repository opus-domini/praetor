package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agent"
	agentruntime "github.com/opus-domini/praetor/internal/agent/runtime"
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

	s.tools.register("exec", "Run a single prompt against an AI agent provider and return the response",
		objectSchema(map[string]any{
			"prompt":   stringProp("The prompt to send to the agent"),
			"provider": stringProp("Agent provider: claude, codex, copilot, gemini, kimi, opencode, openrouter, ollama, or lmstudio (default: codex)"),
			"model":    stringProp("Model name, provider-specific (optional)"),
		}, []string{"prompt"}),
		func(args map[string]any) ([]contentBlock, error) {
			prompt := strings.TrimSpace(argString(args, "prompt"))
			if prompt == "" {
				return nil, fmt.Errorf("prompt is required")
			}
			provider := strings.TrimSpace(argString(args, "provider"))
			if provider == "" {
				provider = "codex"
			}
			model := strings.TrimSpace(argString(args, "model"))

			cfg, _ := config.Load(s.projectDir)
			registry := agentruntime.NewDefaultRegistry(agentruntime.DefaultOptions{
				CodexBin:         cfg.CodexBin,
				ClaudeBin:        cfg.ClaudeBin,
				CopilotBin:       cfg.CopilotBin,
				GeminiBin:        cfg.GeminiBin,
				KimiBin:          cfg.KimiBin,
				OpenCodeBin:      cfg.OpenCodeBin,
				OpenRouterURL:    cfg.OpenRouterURL,
				OpenRouterModel:  cfg.OpenRouterModel,
				OpenRouterKeyEnv: cfg.OpenRouterKeyEnv,
				OllamaURL:        cfg.OllamaURL,
				LMStudioURL:      cfg.LMStudioURL,
				LMStudioKeyEnv:   cfg.LMStudioKeyEnv,
			})

			agentID := agent.Normalize(provider)
			providerAgent, ok := registry.Get(agentID)
			if !ok {
				return nil, fmt.Errorf("unknown provider %q (supported: claude, codex, copilot, gemini, kimi, opencode, openrouter, ollama, lmstudio)", provider)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			workdir := s.projectDir
			if workdir == "" {
				workdir = "."
			}

			response, err := providerAgent.Execute(ctx, agent.ExecuteRequest{
				Prompt:  prompt,
				Model:   model,
				Workdir: workdir,
				OneShot: true,
			})
			if err != nil {
				return nil, fmt.Errorf("agent execution failed: %w", err)
			}

			return jsonContent(map[string]any{
				"output":     response.Output,
				"provider":   provider,
				"model":      response.Model,
				"cost_usd":   response.CostUSD,
				"duration_s": response.DurationS,
			})
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
