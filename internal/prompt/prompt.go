package prompt

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var defaultFS embed.FS

// Engine resolves and renders prompt templates.
// Embedded defaults are always available; an optional overlay directory
// lets projects override individual templates by filename.
type Engine struct {
	templates *template.Template
}

// NewEngine creates an Engine with embedded defaults.
// If overlayDir is non-empty and exists, files in it override the defaults
// by matching filename. A non-existent overlayDir is silently ignored.
func NewEngine(overlayDir string) (*Engine, error) {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}

	base, err := template.New("").
		Option("missingkey=error").
		Funcs(funcMap).
		ParseFS(defaultFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse embedded templates: %w", err)
	}

	overlayDir = strings.TrimSpace(overlayDir)
	if overlayDir != "" {
		if info, statErr := os.Stat(overlayDir); statErr == nil && info.IsDir() {
			if walkErr := filepath.WalkDir(overlayDir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				if !strings.HasSuffix(d.Name(), ".tmpl") {
					return nil
				}
				content, readErr := os.ReadFile(path)
				if readErr != nil {
					return fmt.Errorf("read overlay %s: %w", path, readErr)
				}
				t := base.New(d.Name())
				t.Funcs(funcMap)
				if _, parseErr := t.Parse(string(content)); parseErr != nil {
					return fmt.Errorf("parse overlay %s: %w", path, parseErr)
				}
				return nil
			}); walkErr != nil {
				return nil, fmt.Errorf("overlay templates from %s: %w", overlayDir, walkErr)
			}
		}
	}

	return &Engine{templates: base}, nil
}

// Render executes the named template with the given data and returns
// the result as a trimmed string. The name should omit the "templates/"
// prefix and ".tmpl" suffix (e.g., "executor.system").
func (e *Engine) Render(name string, data any) (string, error) {
	fullName := name + ".tmpl"
	var b strings.Builder
	if err := e.templates.ExecuteTemplate(&b, fullName, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", name, err)
	}
	return strings.TrimSpace(b.String()), nil
}
