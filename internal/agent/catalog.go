package agent

import "sort"

// CatalogEntry holds static metadata about one agent backend.
// This is the single source of truth for agent identity, installation,
// and operational characteristics.
type CatalogEntry struct {
	ID             ID
	DisplayName    string
	Transport      Transport
	Binary         string   // default binary name for CLI agents; empty for REST
	PackageManager string   // "npm", "pip", "go", "brew", or "" if manual/REST
	PackageName    string   // e.g. "@anthropic-ai/claude-code"
	InstallHint    string   // human-readable install instruction
	VersionArgs    []string // args to get version (e.g. ["--version"])
	HealthEndpoint string   // for REST agents: path appended to base URL (e.g. "/api/tags")
	DefaultBaseURL string   // default REST base URL (e.g. "http://127.0.0.1:11434")
	Capabilities   Capabilities
}

// catalog is the authoritative registry of all known agents.
var catalog = map[ID]CatalogEntry{
	Claude: {
		ID:             Claude,
		DisplayName:    "Claude Code",
		Transport:      TransportCLI,
		Binary:         "claude",
		PackageManager: "npm",
		PackageName:    "@anthropic-ai/claude-code",
		InstallHint:    "npm install -g @anthropic-ai/claude-code",
		VersionArgs:    []string{"--version"},
		Capabilities: Capabilities{
			Transport:        TransportCLI,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      true,
			StructuredOutput: true,
		},
	},
	Codex: {
		ID:             Codex,
		DisplayName:    "OpenAI Codex CLI",
		Transport:      TransportCLI,
		Binary:         "codex",
		PackageManager: "npm",
		PackageName:    "@openai/codex",
		InstallHint:    "npm install -g @openai/codex",
		VersionArgs:    []string{"--version"},
		Capabilities: Capabilities{
			Transport:        TransportCLI,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      false,
			StructuredOutput: true,
		},
	},
	Copilot: {
		ID:             Copilot,
		DisplayName:    "GitHub Copilot CLI",
		Transport:      TransportCLI,
		Binary:         "copilot",
		PackageManager: "npm",
		PackageName:    "@githubnext/github-copilot-cli",
		InstallHint:    "npm install -g @githubnext/github-copilot-cli",
		VersionArgs:    []string{"--version"},
		Capabilities: Capabilities{
			Transport:        TransportCLI,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      false,
			StructuredOutput: false,
		},
	},
	Gemini: {
		ID:             Gemini,
		DisplayName:    "Gemini CLI",
		Transport:      TransportCLI,
		Binary:         "gemini",
		PackageManager: "npm",
		PackageName:    "@google/gemini-cli",
		InstallHint:    "npm install -g @google/gemini-cli",
		VersionArgs:    []string{"--version"},
		Capabilities: Capabilities{
			Transport:        TransportCLI,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      true,
			StructuredOutput: false,
		},
	},
	Kimi: {
		ID:             Kimi,
		DisplayName:    "Kimi CLI",
		Transport:      TransportCLI,
		Binary:         "kimi",
		PackageManager: "",
		PackageName:    "",
		InstallHint:    "See https://kimi.ai for installation instructions",
		VersionArgs:    []string{"--version"},
		Capabilities: Capabilities{
			Transport:        TransportCLI,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      true,
			StructuredOutput: false,
		},
	},
	OpenCode: {
		ID:             OpenCode,
		DisplayName:    "OpenCode",
		Transport:      TransportCLI,
		Binary:         "opencode",
		PackageManager: "go",
		PackageName:    "github.com/opencode-ai/opencode",
		InstallHint:    "go install github.com/opencode-ai/opencode@latest",
		VersionArgs:    []string{"--version"},
		Capabilities: Capabilities{
			Transport:        TransportCLI,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      false,
			StructuredOutput: false,
		},
	},
	OpenRouter: {
		ID:             OpenRouter,
		DisplayName:    "OpenRouter API",
		Transport:      TransportREST,
		Binary:         "",
		PackageManager: "",
		PackageName:    "",
		InstallHint:    "Set OPENROUTER_API_KEY environment variable",
		HealthEndpoint: "/api/v1/models",
		DefaultBaseURL: "https://openrouter.ai",
		Capabilities: Capabilities{
			Transport:        TransportREST,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      false,
			StructuredOutput: true,
		},
	},
	Ollama: {
		ID:             Ollama,
		DisplayName:    "Ollama",
		Transport:      TransportREST,
		Binary:         "",
		PackageManager: "",
		PackageName:    "",
		InstallHint:    "See https://ollama.com for installation",
		HealthEndpoint: "/api/tags",
		DefaultBaseURL: "http://127.0.0.1:11434",
		Capabilities: Capabilities{
			Transport:        TransportREST,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      false,
			StructuredOutput: false,
		},
	},
	LMStudio: {
		ID:             LMStudio,
		DisplayName:    "LM Studio",
		Transport:      TransportREST,
		Binary:         "",
		PackageManager: "",
		PackageName:    "",
		InstallHint:    "See https://lmstudio.ai for installation",
		HealthEndpoint: "/v1/models",
		DefaultBaseURL: "http://localhost:1234",
		Capabilities: Capabilities{
			Transport:        TransportREST,
			SupportsPlan:     true,
			SupportsExecute:  true,
			SupportsReview:   true,
			RequiresTTY:      false,
			StructuredOutput: true,
		},
	},
}

// CatalogEntry returns the metadata for a specific agent.
// Returns the entry and true if found, zero value and false otherwise.
func LookupCatalog(id ID) (CatalogEntry, bool) {
	entry, ok := catalog[Normalize(string(id))]
	return entry, ok
}

// AllCatalogEntries returns catalog entries for all known agents, sorted by ID.
func AllCatalogEntries() []CatalogEntry {
	entries := make([]CatalogEntry, 0, len(catalog))
	for _, entry := range catalog {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

// CLICatalogEntries returns only CLI-backed agent entries, sorted by ID.
func CLICatalogEntries() []CatalogEntry {
	var entries []CatalogEntry
	for _, entry := range catalog {
		if entry.Transport == TransportCLI {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

// RESTCatalogEntries returns only REST-backed agent entries, sorted by ID.
func RESTCatalogEntries() []CatalogEntry {
	var entries []CatalogEntry
	for _, entry := range catalog {
		if entry.Transport == TransportREST {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

// CatalogIDs returns sorted IDs of all known agents.
func CatalogIDs() []ID {
	ids := make([]ID, 0, len(catalog))
	for id := range catalog {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
