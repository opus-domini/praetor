package config

import "strings"

// Template generates a commented TOML template from the registry.
// All key=value lines are commented out so the file is safe to create as-is.
func Template() string {
	var b strings.Builder
	groups := GroupedByCategory()
	for i, group := range groups {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("# === ")
		b.WriteString(string(group.Category))
		b.WriteString(" ===\n")
		for _, meta := range group.Keys {
			b.WriteString("# ")
			b.WriteString(meta.Description)
			b.WriteString("\n")
			b.WriteString("# ")
			b.WriteString(formatLine(meta.Key, meta.DefaultValue))
			b.WriteString("\n")
		}
	}
	return b.String()
}
