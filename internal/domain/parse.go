package domain

import (
	"regexp"
	"strings"
)

// ExecutorResult is the parsed final result from executor output.
type ExecutorResult string

const (
	ExecutorResultPass    ExecutorResult = "PASS"
	ExecutorResultFail    ExecutorResult = "FAIL"
	ExecutorResultUnknown ExecutorResult = "UNKNOWN"
)

// ParseErrorClass categorizes output-parse failures by retry semantics.
type ParseErrorClass string

const (
	ParseErrorRecoverable    ParseErrorClass = "recoverable"
	ParseErrorNonRecoverable ParseErrorClass = "non_recoverable"
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
	Hints  []string
}

// ParseReviewDecision parses reviewer output in PASS|reason or FAIL|reason|HINT:... format.
func ParseReviewDecision(output string) ReviewDecision {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}

		parts := strings.Split(trimmed, "|")
		decision := strings.ToUpper(strings.TrimSpace(parts[0]))
		reason := ""
		hints := make([]string, 0)
		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(strings.ToUpper(part), "HINT:") {
				hint := strings.TrimSpace(part[len("HINT:"):])
				if hint != "" {
					hints = append(hints, hint)
				}
				continue
			}
			if part == "" {
				continue
			}
			if reason == "" {
				reason = part
			} else {
				reason += " | " + part
			}
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
			if len(hints) == 0 {
				hints = append(hints, reason)
			}
			return ReviewDecision{Pass: false, Reason: reason, Hints: hints}
		default:
			return ReviewDecision{Pass: false, Reason: "reviewer output must use PASS|... or FAIL|..."}
		}
	}

	return ReviewDecision{Pass: false, Reason: "reviewer output was empty"}
}

// IsReviewerDecisionParseFailure reports whether the reviewer output failed the contract parser.
func IsReviewerDecisionParseFailure(decision ReviewDecision) bool {
	reason := strings.ToLower(strings.TrimSpace(decision.Reason))
	return strings.Contains(reason, "reviewer output must use pass|") ||
		strings.Contains(reason, "reviewer output was empty")
}

// GateResult is one parsed gate evidence line from executor output.
type GateResult struct {
	Name   string
	Status string
	Detail string
}

var gateLinePattern = regexp.MustCompile(`^-\s*([A-Za-z0-9_.-]+):\s*(PASS|FAIL)(.*)$`)

// ParseGateEvidence parses a GATES block from executor output.
func ParseGateEvidence(output string) map[string]GateResult {
	results := make(map[string]GateResult)
	lines := strings.Split(output, "\n")
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "GATES:") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if trimmed == "" {
			break
		}
		match := gateLinePattern.FindStringSubmatch(trimmed)
		if len(match) != 4 {
			if strings.HasPrefix(trimmed, "-") {
				continue
			}
			break
		}
		name := strings.ToLower(strings.TrimSpace(match[1]))
		results[name] = GateResult{
			Name:   name,
			Status: strings.ToUpper(strings.TrimSpace(match[2])),
			Detail: strings.TrimSpace(match[3]),
		}
	}
	return results
}
