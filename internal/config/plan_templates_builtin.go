package config

import (
	"embed"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed plan_templates/*.json
var builtinPlanTemplates embed.FS

func builtinTemplateData(name string) ([]byte, bool) {
	name = normalizeTemplateName(name)
	content, err := builtinPlanTemplates.ReadFile("plan_templates/" + name + ".json")
	if err != nil {
		return nil, false
	}
	return content, true
}

func builtinTemplateNames() []string {
	entries, err := fs.ReadDir(builtinPlanTemplates, "plan_templates")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(filepath.Base(entry.Name()), ".json"))
	}
	sort.Strings(names)
	return names
}
