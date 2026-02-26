package domain

import "strings"

// ExecutorResult is the parsed final result from executor output.
type ExecutorResult string

const (
	ExecutorResultPass    ExecutorResult = "PASS"
	ExecutorResultFail    ExecutorResult = "FAIL"
	ExecutorResultUnknown ExecutorResult = "UNKNOWN"
)

// ParseExecutorResult parses the RESULT line from executor output.
func ParseExecutorResult(output string) ExecutorResult {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "RESULT:") {
			value := strings.TrimSpace(strings.TrimPrefix(upper, "RESULT:"))
			switch value {
			case "PASS":
				return ExecutorResultPass
			case "FAIL":
				return ExecutorResultFail
			default:
				return ExecutorResultUnknown
			}
		}
	}
	return ExecutorResultUnknown
}

// ReviewDecision is the parsed reviewer decision.
type ReviewDecision struct {
	Pass   bool
	Reason string
}

// ParseReviewDecision parses reviewer output in PASS|reason or FAIL|reason format.
func ParseReviewDecision(output string) ReviewDecision {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, "|", 2)
		decision := strings.ToUpper(strings.TrimSpace(parts[0]))
		reason := ""
		if len(parts) == 2 {
			reason = strings.TrimSpace(parts[1])
		}

		switch decision {
		case "PASS":
			if reason == "" {
				reason = "review passed"
			}
			return ReviewDecision{Pass: true, Reason: reason}
		case "FAIL":
			if reason == "" {
				reason = "review failed"
			}
			return ReviewDecision{Pass: false, Reason: reason}
		default:
			return ReviewDecision{Pass: false, Reason: "reviewer output must use PASS|... or FAIL|..."}
		}
	}

	return ReviewDecision{Pass: false, Reason: "reviewer output was empty"}
}
