package domain

import (
	"encoding/json"
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

// executorStructuredOutput is the JSON schema for executor structured output.
type executorStructuredOutput struct {
	Result  string `json:"result"`
	Summary string `json:"summary"`
}

// ParseExecutorResult parses executor output, trying JSON structured output first,
// then falling back to the text-based RESULT: line format.
func ParseExecutorResult(output string) ExecutorResult {
	if result := parseExecutorJSON(output); result != ExecutorResultUnknown {
		return result
	}
	return parseExecutorText(output)
}

func parseExecutorJSON(output string) ExecutorResult {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var s executorStructuredOutput
		if json.Unmarshal([]byte(line), &s) != nil {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(s.Result)) {
		case "PASS":
			return ExecutorResultPass
		case "FAIL":
			return ExecutorResultFail
		}
	}
	return ExecutorResultUnknown
}

func parseExecutorText(output string) ExecutorResult {
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

// reviewerStructuredOutput is the JSON schema for reviewer structured output.
type reviewerStructuredOutput struct {
	Decision string   `json:"decision"`
	Reason   string   `json:"reason"`
	Hints    []string `json:"hints"`
}

// ParseReviewDecision parses reviewer output, trying JSON structured output first,
// then falling back to the text-based PASS|reason or FAIL|reason|HINT:... format.
// For text mode, it scans all lines and uses the last PASS|... or FAIL|... line,
// allowing reviewers to emit analysis text before the final verdict.
func ParseReviewDecision(output string) ReviewDecision {
	if d := parseReviewJSON(output); d != nil {
		return *d
	}
	return parseReviewText(output)
}

func parseReviewJSON(output string) *ReviewDecision {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var s reviewerStructuredOutput
		if json.Unmarshal([]byte(line), &s) != nil {
			continue
		}
		decision := strings.ToUpper(strings.TrimSpace(s.Decision))
		reason := strings.TrimSpace(s.Reason)
		switch decision {
		case "PASS":
			if reason == "" {
				reason = "review passed"
			}
			return &ReviewDecision{Pass: true, Reason: reason}
		case "FAIL":
			if reason == "" {
				reason = "review failed"
			}
			hints := make([]string, 0, len(s.Hints))
			for _, h := range s.Hints {
				if h = strings.TrimSpace(h); h != "" {
					hints = append(hints, h)
				}
			}
			if len(hints) == 0 {
				hints = append(hints, reason)
			}
			return &ReviewDecision{Pass: false, Reason: reason, Hints: hints}
		}
	}
	return nil
}

func parseReviewText(output string) ReviewDecision {
	var last *ReviewDecision
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}

		parts := strings.Split(trimmed, "|")
		decision := strings.ToUpper(strings.TrimSpace(parts[0]))

		switch decision {
		case "PASS", "FAIL":
		default:
			continue
		}

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
			last = &ReviewDecision{Pass: true, Reason: reason}
		case "FAIL":
			if reason == "" {
				reason = "review failed"
			}
			if len(hints) == 0 {
				hints = append(hints, reason)
			}
			last = &ReviewDecision{Pass: false, Reason: reason, Hints: hints}
		}
	}

	if last != nil {
		return *last
	}
	if strings.TrimSpace(output) == "" {
		return ReviewDecision{Pass: false, Reason: "reviewer output was empty"}
	}
	return ReviewDecision{Pass: false, Reason: "reviewer output must use PASS|... or FAIL|..."}
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

// executorGatesOutput matches the gates field from executor structured output.
type executorGatesOutput struct {
	Tests     string `json:"tests"`
	Lint      string `json:"lint"`
	Standards string `json:"standards"`
}

// executorFullStructuredOutput combines result + gates for gate extraction.
type executorFullStructuredOutput struct {
	Result string               `json:"result"`
	Gates  *executorGatesOutput `json:"gates,omitempty"`
}

// ParseGateEvidence parses gate evidence from executor output,
// trying JSON structured output first, then text-based GATES: block.
func ParseGateEvidence(output string) map[string]GateResult {
	if gates := parseGateJSON(output); len(gates) > 0 {
		return gates
	}
	return parseGateText(output)
}

func parseGateJSON(output string) map[string]GateResult {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var s executorFullStructuredOutput
		if json.Unmarshal([]byte(line), &s) != nil {
			continue
		}
		if s.Gates == nil {
			continue
		}
		results := make(map[string]GateResult)
		for _, entry := range []struct{ name, status string }{
			{"tests", s.Gates.Tests},
			{"lint", s.Gates.Lint},
			{"standards", s.Gates.Standards},
		} {
			status := strings.ToUpper(strings.TrimSpace(entry.status))
			if status == "PASS" || status == "FAIL" {
				results[entry.name] = GateResult{Name: entry.name, Status: status}
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	return nil
}

func parseGateText(output string) map[string]GateResult {
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
