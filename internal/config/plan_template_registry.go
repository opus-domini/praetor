package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/state"
)

const builtinTemplatePrefix = "builtin:"

type TemplateInfo struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

func FindTemplate(name string, projectRoot string) (string, error) {
	name = normalizeTemplateName(name)
	if name == "" {
		return "", fmt.Errorf("template name is required")
	}

	for _, base := range templateSearchDirs(projectRoot) {
		path := filepath.Join(base, name+".json")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if _, ok := builtinTemplateData(name); ok {
		return builtinTemplatePrefix + name, nil
	}
	return "", fmt.Errorf("template %q not found", name)
}

func ListTemplates(projectRoot string) ([]TemplateInfo, error) {
	seen := make(map[string]struct{})
	infos := make([]TemplateInfo, 0)

	for _, base := range templateSearchDirs(projectRoot) {
		matches, err := filepath.Glob(filepath.Join(base, "*.json"))
		if err != nil {
			return nil, fmt.Errorf("list templates in %s: %w", base, err)
		}
		for _, match := range matches {
			name := strings.TrimSuffix(filepath.Base(match), ".json")
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			infos = append(infos, TemplateInfo{
				Name:   name,
				Source: templateSourceLabel(base, projectRoot),
				Path:   match,
			})
		}
	}

	for _, name := range builtinTemplateNames() {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		infos = append(infos, TemplateInfo{
			Name:   name,
			Source: "builtin",
			Path:   builtinTemplatePrefix + name,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos, nil
}

func RenderTemplate(tmplPath string, vars map[string]string) (*domain.Plan, error) {
	content, err := loadTemplateContent(tmplPath)
	if err != nil {
		return nil, err
	}

	renderer, err := template.New(filepath.Base(strings.TrimSpace(tmplPath))).
		Option("missingkey=error").
		Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", tmplPath, err)
	}

	payload := make(map[string]string, len(vars))
	for key, value := range vars {
		payload[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	var rendered bytes.Buffer
	if err := renderer.Execute(&rendered, payload); err != nil {
		return nil, fmt.Errorf("render template %s: %w", tmplPath, err)
	}

	plan, err := domain.ParsePlanStrict(rendered.Bytes())
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func templateSearchDirs(projectRoot string) []string {
	dirs := make([]string, 0, 2)
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot != "" {
		dirs = append(dirs, filepath.Join(projectRoot, ".praetor", "templates"))
	}
	if home, err := state.DefaultHome(); err == nil && strings.TrimSpace(home) != "" {
		dirs = append(dirs, filepath.Join(home, "templates"))
	}
	return dirs
}

func templateSourceLabel(base, projectRoot string) string {
	projectRoot = strings.TrimSpace(projectRoot)
	base = strings.TrimSpace(base)
	if projectRoot != "" && strings.HasPrefix(base, filepath.Join(projectRoot, ".praetor", "templates")) {
		return "project"
	}
	return "global"
}

func normalizeTemplateName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".json")
	return name
}

func loadTemplateContent(tmplPath string) ([]byte, error) {
	tmplPath = strings.TrimSpace(tmplPath)
	if strings.HasPrefix(tmplPath, builtinTemplatePrefix) {
		name := strings.TrimPrefix(tmplPath, builtinTemplatePrefix)
		content, ok := builtinTemplateData(name)
		if !ok {
			return nil, fmt.Errorf("builtin template %q not found", name)
		}
		return content, nil
	}
	content, err := os.ReadFile(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("read template %s: %w", tmplPath, err)
	}
	return content, nil
}
