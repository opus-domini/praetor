package adapters

import (
	"errors"
	"strings"

	agent "github.com/opus-domini/praetor/internal/agent"
)

func ComposePrompt(systemPrompt, prompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	prompt = strings.TrimSpace(prompt)
	if systemPrompt == "" {
		return prompt
	}
	if prompt == "" {
		return systemPrompt
	}
	return systemPrompt + "\n\n" + prompt
}

func ExtractJSONObject(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty output")
	}
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start < 0 || end < 0 || end <= start {
		return "", errors.New("json object not found")
	}
	return strings.TrimSpace(input[start : end+1]), nil
}

func ParseReview(output string) (agent.ReviewDecision, string) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "DECISION:") {
			value := strings.TrimSpace(strings.TrimPrefix(upper, "DECISION:"))
			switch value {
			case "PASS":
				return agent.DecisionPass, ""
			case "FAIL":
				reason := ""
				if idx := strings.Index(strings.ToUpper(output), "REASON:"); idx >= 0 {
					reason = strings.TrimSpace(output[idx+len("REASON:"):])
				}
				return agent.DecisionFail, reason
			}
		}
	}
	return agent.DecisionUnknown, ""
}

func TailText(input string, maxLines int) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "no stderr output"
	}
	lines := strings.Split(input, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, " | ")
}
