package providers

import "strings"

// ID identifies one supported provider implementation.
type ID string

const (
	Codex  ID = "codex"
	Claude ID = "claude"
)

// Normalize canonicalizes provider identifiers for validation and routing.
func Normalize(raw string) ID {
	return ID(strings.ToLower(strings.TrimSpace(raw)))
}

// IsSupported reports whether id maps to a built-in provider.
func IsSupported(id ID) bool {
	switch id {
	case Codex, Claude:
		return true
	default:
		return false
	}
}
