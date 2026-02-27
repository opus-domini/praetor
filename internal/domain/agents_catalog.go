package domain

import "strings"

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

var agentCapabilities = map[Agent]AgentCapabilities{
	AgentCodex: {
		Transport:       AgentTransportCLI,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     false,
		RequiresBinary:  true,
		TMUXCompatible:  true,
	},
	AgentClaude: {
		Transport:       AgentTransportCLI,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     true,
		RequiresBinary:  true,
		TMUXCompatible:  true,
	},
	AgentCopilot: {
		Transport:       AgentTransportCLI,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     false,
		RequiresBinary:  true,
		TMUXCompatible:  true,
	},
	AgentGemini: {
		Transport:       AgentTransportCLI,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     true,
		RequiresBinary:  true,
		TMUXCompatible:  true,
	},
	AgentKimi: {
		Transport:       AgentTransportCLI,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     true,
		RequiresBinary:  true,
		TMUXCompatible:  true,
	},
	AgentOpenCode: {
		Transport:       AgentTransportCLI,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     false,
		RequiresBinary:  true,
		TMUXCompatible:  true,
	},
	AgentOpenRouter: {
		Transport:       AgentTransportREST,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     false,
		RequiresBinary:  false,
		TMUXCompatible:  true,
	},
	AgentOllama: {
		Transport:       AgentTransportREST,
		SupportsPlan:    true,
		SupportsExecute: true,
		SupportsReview:  true,
		RequiresTTY:     false,
		RequiresBinary:  false,
		TMUXCompatible:  true,
	},
}

// CapabilitiesForAgent returns static capabilities for the given agent.
func CapabilitiesForAgent(agent Agent) (AgentCapabilities, bool) {
	caps, ok := agentCapabilities[NormalizeAgent(agent)]
	return caps, ok
}

// AgentSupportsTMUX reports whether an agent can be used when runner mode is tmux.
func AgentSupportsTMUX(agent Agent) bool {
	agent = NormalizeAgent(agent)
	if agent == "" || agent == AgentNone {
		return true
	}
	caps, ok := CapabilitiesForAgent(agent)
	return ok && caps.TMUXCompatible
}

// AgentRequiresBinary reports whether a provider relies on a local executable.
func AgentRequiresBinary(agent Agent) bool {
	caps, ok := CapabilitiesForAgent(agent)
	return ok && caps.RequiresBinary
}

// AgentBinary returns the configured binary for CLI-backed providers.
func AgentBinary(opts RunnerOptions, agent Agent) (string, bool) {
	switch NormalizeAgent(agent) {
	case AgentCodex:
		return strings.TrimSpace(opts.CodexBin), true
	case AgentClaude:
		return strings.TrimSpace(opts.ClaudeBin), true
	case AgentCopilot:
		return strings.TrimSpace(opts.CopilotBin), true
	case AgentGemini:
		return strings.TrimSpace(opts.GeminiBin), true
	case AgentKimi:
		return strings.TrimSpace(opts.KimiBin), true
	case AgentOpenCode:
		return strings.TrimSpace(opts.OpenCodeBin), true
	default:
		return "", false
	}
}

// AgentDisplayName returns a stable human-readable name.
func AgentDisplayName(agent Agent) string {
	agent = NormalizeAgent(agent)
	if agent == "" {
		return ""
	}
	return string(agent)
}
