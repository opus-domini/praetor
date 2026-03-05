package config

// KeyType describes the expected value type for a config key.
type KeyType int

const (
	KeyTypeString KeyType = iota
	KeyTypeInt
	KeyTypeBool
	KeyTypeDuration
)

// Category groups config keys for display.
type Category string

const (
	CategoryAgents   Category = "Agents"
	CategoryLimits   Category = "Limits"
	CategoryRuntime  Category = "Runtime"
	CategoryBinaries Category = "Binaries"
	CategoryREST     Category = "REST"
	CategoryFallback Category = "Fallback"
)

// CategoryOrder defines the display order for categories.
var CategoryOrder = []Category{
	CategoryAgents,
	CategoryLimits,
	CategoryRuntime,
	CategoryBinaries,
	CategoryREST,
	CategoryFallback,
}

// KeyMeta holds metadata for a single config key.
type KeyMeta struct {
	Key          string
	DefaultValue string
	Type         KeyType
	Category     Category
	Description  string
}

// Registry is the ordered list of all config keys with their defaults.
var Registry = []KeyMeta{
	// Agents
	{Key: "executor", DefaultValue: "codex", Type: KeyTypeString, Category: CategoryAgents, Description: "Default executor agent"},
	{Key: "reviewer", DefaultValue: "claude", Type: KeyTypeString, Category: CategoryAgents, Description: "Default reviewer agent"},
	{Key: "planner", DefaultValue: "claude", Type: KeyTypeString, Category: CategoryAgents, Description: "Planner agent for macro-planning"},

	// Limits
	{Key: "max-retries", DefaultValue: "3", Type: KeyTypeInt, Category: CategoryLimits, Description: "Maximum retries per task (must be > 0)"},
	{Key: "max-iterations", DefaultValue: "0", Type: KeyTypeInt, Category: CategoryLimits, Description: "Maximum loop iterations (0 = unlimited)"},
	{Key: "max-transitions", DefaultValue: "0", Type: KeyTypeInt, Category: CategoryLimits, Description: "Maximum FSM state transitions (0 = unlimited)"},
	{Key: "keep-last-runs", DefaultValue: "20", Type: KeyTypeInt, Category: CategoryLimits, Description: "Keep only the most recent N runs (0 = no pruning)"},
	{Key: "timeout", DefaultValue: "0s", Type: KeyTypeDuration, Category: CategoryLimits, Description: "Run timeout (e.g. 30m, 2h)"},

	// Runtime
	{Key: "runner", DefaultValue: "tmux", Type: KeyTypeString, Category: CategoryRuntime, Description: "Runner mode: tmux, pty, or direct"},
	{Key: "isolation", DefaultValue: "worktree", Type: KeyTypeString, Category: CategoryRuntime, Description: "Isolation mode: worktree or off"},
	{Key: "no-review", DefaultValue: "false", Type: KeyTypeBool, Category: CategoryRuntime, Description: "Skip the reviewer gate and auto-approve"},
	{Key: "no-color", DefaultValue: "false", Type: KeyTypeBool, Category: CategoryRuntime, Description: "Disable colored output"},
	{Key: "hook", DefaultValue: "", Type: KeyTypeString, Category: CategoryRuntime, Description: "Script to run after executor, before reviewer"},
	{Key: "gate-tests-cmd", DefaultValue: "go test ./...", Type: KeyTypeString, Category: CategoryRuntime, Description: "Host command for tests quality gate"},
	{Key: "gate-lint-cmd", DefaultValue: "golangci-lint run", Type: KeyTypeString, Category: CategoryRuntime, Description: "Host command for lint quality gate"},
	{Key: "gate-standards-cmd", DefaultValue: "go test ./... && golangci-lint run", Type: KeyTypeString, Category: CategoryRuntime, Description: "Host command for standards quality gate"},

	// Binaries
	{Key: "codex-bin", DefaultValue: "codex", Type: KeyTypeString, Category: CategoryBinaries, Description: "Codex binary path or name"},
	{Key: "claude-bin", DefaultValue: "claude", Type: KeyTypeString, Category: CategoryBinaries, Description: "Claude binary path or name"},
	{Key: "copilot-bin", DefaultValue: "copilot", Type: KeyTypeString, Category: CategoryBinaries, Description: "Copilot binary path or name"},
	{Key: "gemini-bin", DefaultValue: "gemini", Type: KeyTypeString, Category: CategoryBinaries, Description: "Gemini CLI binary path or name"},
	{Key: "kimi-bin", DefaultValue: "kimi", Type: KeyTypeString, Category: CategoryBinaries, Description: "Kimi binary path or name"},
	{Key: "opencode-bin", DefaultValue: "opencode", Type: KeyTypeString, Category: CategoryBinaries, Description: "OpenCode binary path or name"},

	// REST
	{Key: "openrouter-url", DefaultValue: "https://openrouter.ai/api/v1", Type: KeyTypeString, Category: CategoryREST, Description: "OpenRouter base URL"},
	{Key: "openrouter-model", DefaultValue: "openai/gpt-4o-mini", Type: KeyTypeString, Category: CategoryREST, Description: "Default OpenRouter model"},
	{Key: "openrouter-api-key-env", DefaultValue: "OPENROUTER_API_KEY", Type: KeyTypeString, Category: CategoryREST, Description: "Env var containing OpenRouter API key"},
	{Key: "ollama-url", DefaultValue: "http://127.0.0.1:11434", Type: KeyTypeString, Category: CategoryREST, Description: "Ollama base URL for REST requests"},
	{Key: "ollama-model", DefaultValue: "llama3", Type: KeyTypeString, Category: CategoryREST, Description: "Default Ollama model"},
	{Key: "lmstudio-url", DefaultValue: "http://localhost:1234", Type: KeyTypeString, Category: CategoryREST, Description: "LM Studio base URL for REST requests"},
	{Key: "lmstudio-model", DefaultValue: "", Type: KeyTypeString, Category: CategoryREST, Description: "Default LM Studio model"},
	{Key: "lmstudio-api-key-env", DefaultValue: "LMSTUDIO_API_KEY", Type: KeyTypeString, Category: CategoryREST, Description: "Env var containing LM Studio API key (optional)"},

	// Fallback
	{Key: "fallback", DefaultValue: "", Type: KeyTypeString, Category: CategoryFallback, Description: "Per-agent fallback (primary -> fallback)"},
	{Key: "fallback-on-transient", DefaultValue: "", Type: KeyTypeString, Category: CategoryFallback, Description: "Global fallback agent for transient errors"},
	{Key: "fallback-on-auth", DefaultValue: "", Type: KeyTypeString, Category: CategoryFallback, Description: "Global fallback agent for auth errors"},
}

// CategoryGroup holds a category and its keys for grouped display.
type CategoryGroup struct {
	Category Category
	Keys     []KeyMeta
}

// LookupMeta returns the metadata for a given config key.
func LookupMeta(key string) (KeyMeta, bool) {
	for _, m := range Registry {
		if m.Key == key {
			return m, true
		}
	}
	return KeyMeta{}, false
}

// GroupedByCategory returns registry entries grouped by category in display order.
func GroupedByCategory() []CategoryGroup {
	groups := make(map[Category][]KeyMeta)
	for _, m := range Registry {
		groups[m.Category] = append(groups[m.Category], m)
	}
	var result []CategoryGroup
	for _, cat := range CategoryOrder {
		if keys, ok := groups[cat]; ok {
			result = append(result, CategoryGroup{Category: cat, Keys: keys})
		}
	}
	return result
}
