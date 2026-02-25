package config

import (
	"fmt"
	"strconv"
	"strings"
)

// parse reads a flat TOML-compatible config file.
// Returns a map of section -> key -> value.
// The global (top-level) section has key "".
// Project sections use [projects."<path>"] syntax.
func parse(input string) (map[string]map[string]string, error) {
	sections := map[string]map[string]string{
		"": {},
	}
	currentSection := ""

	for lineNum, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section, err := parseSectionHeader(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
			}
			currentSection = section
			if _, ok := sections[currentSection]; !ok {
				sections[currentSection] = map[string]string{}
			}
			continue
		}

		key, value, err := parseKeyValue(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
		sections[currentSection][key] = value
	}
	return sections, nil
}

func parseSectionHeader(line string) (string, error) {
	inner := strings.TrimSpace(line[1 : len(line)-1])
	if !strings.HasPrefix(inner, "projects.") {
		return inner, nil
	}

	path := strings.TrimPrefix(inner, "projects.")
	path = strings.Trim(path, "\"")
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty project path in section header")
	}
	return path, nil
}

func parseKeyValue(line string) (string, string, error) {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return "", "", fmt.Errorf("expected key = value, got %q", line)
	}

	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])

	if key == "" {
		return "", "", fmt.Errorf("empty key in %q", line)
	}

	// Strip surrounding quotes from string values
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}

	return key, value, nil
}

func intPtr(s string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &n
}

func boolPtr(s string) *bool {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "true", "1", "yes":
		b := true
		return &b
	case "false", "0", "no":
		b := false
		return &b
	}
	return nil
}
