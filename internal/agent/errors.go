package agent

import "strings"

// ErrorClass categorizes agent execution errors for fallback routing.
type ErrorClass string

const (
	ErrorTransient   ErrorClass = "transient"
	ErrorAuth        ErrorClass = "auth"
	ErrorRateLimit   ErrorClass = "rate_limit"
	ErrorUnsupported ErrorClass = "unsupported"
	ErrorUnknown     ErrorClass = "unknown"
)

// ClassifyError inspects an error's message to determine its class.
// All adapters use plain fmt.Errorf, so string matching is sufficient.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorUnknown
	}
	msg := strings.ToLower(err.Error())

	if matchesAny(msg, "429", "rate limit", "rate_limit", "too many requests") {
		return ErrorRateLimit
	}
	if matchesAny(msg, "401", "403", "api key", "unauthorized", "forbidden", "authentication") {
		return ErrorAuth
	}
	if matchesAny(msg, "connection refused", "timeout", "502", "503", "504",
		"temporary failure", "network unreachable", "no such host",
		"connection reset", "broken pipe", "eof") {
		return ErrorTransient
	}
	if matchesAny(msg, "unsupported", "not implemented", "not supported") {
		return ErrorUnsupported
	}
	return ErrorUnknown
}

func matchesAny(msg string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
