package agents

import "net/http"

// DefaultOptions configures the built-in registry.
type DefaultOptions struct {
	CodexBin    string
	ClaudeBin   string
	GeminiBin   string
	OllamaURL   string
	OllamaModel string
	Runner      CommandRunner
	HTTPClient  *http.Client
}

// NewDefaultRegistry creates a registry with built-in CLI and REST adapters.
func NewDefaultRegistry(opts DefaultOptions) *Registry {
	runner := opts.Runner
	if runner == nil {
		runner = NewExecCommandRunner()
	}

	registry := NewRegistry()
	_ = registry.Register(NewCodexCLI(opts.CodexBin, runner))
	_ = registry.Register(NewClaudeCLI(opts.ClaudeBin, runner))
	_ = registry.Register(NewGeminiCLI(opts.GeminiBin, runner))
	_ = registry.Register(NewOllamaREST(opts.OllamaURL, opts.OllamaModel, opts.HTTPClient))
	return registry
}
