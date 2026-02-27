package domain

import (
	"strings"

	agent "github.com/opus-domini/praetor/internal/agent"
)

// AgentTransport identifies how a provider is reached.
type AgentTransport string

const (
	AgentTransportCLI  AgentTransport = "cli"
	AgentTransportREST AgentTransport = "rest"
)

// AgentCapabilities centralizes operational characteristics per provider.
type AgentCapabilities struct {
	Transport       AgentTransport
	SupportsPlan    bool
	SupportsExecute bool
	SupportsReview  bool
	RequiresTTY     bool
	RequiresBinary  bool
	TMUXCompatible  bool
}

// capabilitiesFromCatalog derives domain capabilities from the canonical agent catalog entry.
func capabilitiesFromCatalog(entry agent.CatalogEntry) AgentCapabilities {
	return AgentCapabilities{
		Transport:       AgentTransport(entry.Transport),
		SupportsPlan:    entry.Capabilities.SupportsPlan,
		SupportsExecute: entry.Capabilities.SupportsExecute,
		SupportsReview:  entry.Capabilities.SupportsReview,
		RequiresTTY:     entry.Capabilities.RequiresTTY,
		RequiresBinary:  entry.Transport == agent.TransportCLI,
		TMUXCompatible:  true, // All currently supported agents are tmux-compatible.
	}
}

// CapabilitiesForAgent returns static capabilities for the given agent,
// derived from the canonical agent catalog.
func CapabilitiesForAgent(a Agent) (AgentCapabilities, bool) {
	entry, ok := agent.LookupCatalog(agent.ID(NormalizeAgent(a)))
	if !ok {
		return AgentCapabilities{}, false
	}
	return capabilitiesFromCatalog(entry), true
}

// AgentSupportsTMUX reports whether an agent can be used when runner mode is tmux.
func AgentSupportsTMUX(a Agent) bool {
	a = NormalizeAgent(a)
	if a == "" || a == AgentNone {
		return true
	}
	caps, ok := CapabilitiesForAgent(a)
	return ok && caps.TMUXCompatible
}

// AgentRequiresBinary reports whether a provider relies on a local executable.
func AgentRequiresBinary(a Agent) bool {
	caps, ok := CapabilitiesForAgent(a)
	return ok && caps.RequiresBinary
}

// AgentBinary returns the configured binary for CLI-backed providers.
// It resolves the binary from RunnerOptions using the agent catalog's default binary name.
func AgentBinary(opts RunnerOptions, a Agent) (string, bool) {
	normalized := NormalizeAgent(a)

	// Check config overrides first.
	override := agentBinaryFromOpts(opts, normalized)
	if override != "" {
		return override, true
	}

	// Fall back to the catalog default binary.
	entry, ok := agent.LookupCatalog(agent.ID(normalized))
	if !ok || entry.Transport != agent.TransportCLI {
		return "", false
	}
	return entry.Binary, true
}

// agentBinaryFromOpts resolves a configured binary override from RunnerOptions.
func agentBinaryFromOpts(opts RunnerOptions, a Agent) string {
	switch a {
	case AgentCodex:
		return strings.TrimSpace(opts.CodexBin)
	case AgentClaude:
		return strings.TrimSpace(opts.ClaudeBin)
	case AgentCopilot:
		return strings.TrimSpace(opts.CopilotBin)
	case AgentGemini:
		return strings.TrimSpace(opts.GeminiBin)
	case AgentKimi:
		return strings.TrimSpace(opts.KimiBin)
	case AgentOpenCode:
		return strings.TrimSpace(opts.OpenCodeBin)
	default:
		return ""
	}
}

// AgentDisplayName returns a stable human-readable name from the catalog.
func AgentDisplayName(a Agent) string {
	a = NormalizeAgent(a)
	if a == "" {
		return ""
	}
	entry, ok := agent.LookupCatalog(agent.ID(a))
	if !ok {
		return string(a)
	}
	return entry.DisplayName
}
