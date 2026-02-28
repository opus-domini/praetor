package agent

// FallbackPolicy configures agent fallback behavior when errors occur.
type FallbackPolicy struct {
	Mappings    map[ID]ID // per-agent overrides: primary → fallback
	OnTransient ID        // global fallback for transient errors
	OnAuth      ID        // global fallback for auth errors
}

// Resolve returns the fallback agent for a given primary agent and error class.
// It checks per-agent mappings first, then global class-based fallbacks.
func (p FallbackPolicy) Resolve(primary ID, class ErrorClass) (ID, bool) {
	if fb, ok := p.Mappings[primary]; ok && fb != "" {
		return fb, true
	}
	switch class {
	case ErrorTransient:
		if p.OnTransient != "" {
			return p.OnTransient, true
		}
	case ErrorAuth:
		if p.OnAuth != "" {
			return p.OnAuth, true
		}
	}
	return "", false
}

// IsEmpty reports whether this policy has no fallback configuration.
func (p FallbackPolicy) IsEmpty() bool {
	return len(p.Mappings) == 0 && p.OnTransient == "" && p.OnAuth == ""
}
