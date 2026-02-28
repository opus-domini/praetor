package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SetValue sets a config key in the given config file. An empty section targets
// the global (top-level) scope; a non-empty section targets [projects."<section>"].
func SetValue(configPath, section, key, value string) error {
	if !IsAllowedKey(key) {
		return fmt.Errorf("unknown config key %q", key)
	}
	if err := ValidateValue(key, value); err != nil {
		return err
	}

	data, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read config: %w", err)
	}

	var lines []string
	if len(data) > 0 {
		lines = strings.Split(string(data), "\n")
	}

	formatted := formatLine(key, value)
	lines = setInLines(lines, section, key, formatted)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return os.WriteFile(configPath, []byte(output), 0o644)
}

// ValidateValue validates a value against the key's expected type.
func ValidateValue(key, value string) error {
	meta, ok := LookupMeta(key)
	if !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	switch meta.Type {
	case KeyTypeInt:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("key %q: invalid integer %q", key, value)
		}
		if key == "max-retries" && n <= 0 {
			return fmt.Errorf("key %q: must be greater than zero", key)
		}
		if n < 0 {
			return fmt.Errorf("key %q: cannot be negative", key)
		}
	case KeyTypeBool:
		if _, err := parseBool(value); err != nil {
			return fmt.Errorf("key %q: %w", key, err)
		}
	case KeyTypeDuration:
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("key %q: invalid duration %q", key, value)
		}
		if d < 0 {
			return fmt.Errorf("key %q: cannot be negative", key)
		}
	}
	return nil
}

func formatLine(key, value string) string {
	meta, ok := LookupMeta(key)
	if !ok {
		return key + " = " + quoteString(value)
	}
	switch meta.Type {
	case KeyTypeInt, KeyTypeBool:
		return key + " = " + value
	default:
		return key + " = " + quoteString(value)
	}
}

func quoteString(v string) string {
	return `"` + v + `"`
}

func setInLines(lines []string, section, key, formatted string) []string {
	if section == "" {
		return setInGlobal(lines, key, formatted)
	}
	return setInProject(lines, section, key, formatted)
}

func setInGlobal(lines []string, key, formatted string) []string {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		if isKeyLine(trimmed, key) {
			lines[i] = formatted
			return lines
		}
	}

	insertAt := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			insertAt = i
			break
		}
	}
	return insertLineAt(lines, insertAt, formatted)
}

func setInProject(lines []string, section, key, formatted string) []string {
	sectionHeader := `[projects."` + section + `"]`

	sectionStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == sectionHeader {
			sectionStart = i
			break
		}
	}

	if sectionStart < 0 {
		result := make([]string, 0, len(lines)+3)
		result = append(result, lines...)
		if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) != "" {
			result = append(result, "")
		}
		result = append(result, sectionHeader)
		result = append(result, formatted)
		return result
	}

	sectionEnd := len(lines)
	for i := sectionStart + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") {
			sectionEnd = i
			break
		}
		if isKeyLine(trimmed, key) {
			lines[i] = formatted
			return lines
		}
	}

	return insertLineAt(lines, sectionEnd, formatted)
}

func isKeyLine(trimmed, key string) bool {
	if strings.HasPrefix(trimmed, "#") || trimmed == "" {
		return false
	}
	idx := strings.Index(trimmed, "=")
	if idx < 0 {
		return false
	}
	return strings.TrimSpace(trimmed[:idx]) == key
}

func insertLineAt(lines []string, pos int, line string) []string {
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:pos]...)
	result = append(result, line)
	result = append(result, lines[pos:]...)
	return result
}
