package runtime

import (
	"net/http"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/agent/adapters"
	"github.com/opus-domini/praetor/internal/agent/runner"
)

// DefaultOptions configures the built-in registry.
type DefaultOptions struct {
	CodexBin         string
	ClaudeBin        string
	CopilotBin       string
	GeminiBin        string
	KimiBin          string
	OpenCodeBin      string
	OpenRouterURL    string
	OpenRouterModel  string
	OpenRouterKeyEnv string
	OllamaURL        string
	OllamaModel      string
	LMStudioURL      string
	LMStudioModel    string
	LMStudioKeyEnv   string
	Runner           runner.CommandRunner
	HTTPClient       *http.Client
}

// NewDefaultRegistry creates a registry with built-in CLI and REST adapters.
func NewDefaultRegistry(opts DefaultOptions) *agent.Registry {
	commandRunner := opts.Runner
	if commandRunner == nil {
		commandRunner = runner.NewExecCommandRunner()
	}

	registry := agent.NewRegistry()
	_ = registry.Register(adapters.NewCodexCLI(opts.CodexBin, commandRunner))
	_ = registry.Register(adapters.NewClaudeCLI(opts.ClaudeBin, commandRunner))
	_ = registry.Register(adapters.NewCopilotCLI(opts.CopilotBin, commandRunner))
	_ = registry.Register(adapters.NewGeminiCLI(opts.GeminiBin, commandRunner))
	_ = registry.Register(adapters.NewKimiCLI(opts.KimiBin, commandRunner))
	_ = registry.Register(adapters.NewOpenCodeCLI(opts.OpenCodeBin, commandRunner))
	_ = registry.Register(adapters.NewOpenRouterREST(opts.OpenRouterURL, opts.OpenRouterModel, opts.OpenRouterKeyEnv, opts.HTTPClient))
	_ = registry.Register(adapters.NewOllamaREST(opts.OllamaURL, opts.OllamaModel, opts.HTTPClient))
	_ = registry.Register(adapters.NewLMStudioREST(opts.LMStudioURL, opts.LMStudioModel, opts.LMStudioKeyEnv, opts.HTTPClient))
	return registry
}
