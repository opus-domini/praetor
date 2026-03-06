package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindTemplateRespectsPriorityOrder(t *testing.T) {
	projectRoot := t.TempDir()
	globalHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", globalHome)

	projectDir := filepath.Join(projectRoot, ".praetor", "templates")
	globalDir := filepath.Join(globalHome, "templates")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project templates dir: %v", err)
	}
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("create global templates dir: %v", err)
	}

	projectTemplate := filepath.Join(projectDir, "shared.json")
	globalTemplate := filepath.Join(globalDir, "shared.json")
	if err := os.WriteFile(projectTemplate, []byte(`{"name":"project","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}`), 0o644); err != nil {
		t.Fatalf("write project template: %v", err)
	}
	if err := os.WriteFile(globalTemplate, []byte(`{"name":"global","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}`), 0o644); err != nil {
		t.Fatalf("write global template: %v", err)
	}

	path, err := FindTemplate("shared", projectRoot)
	if err != nil {
		t.Fatalf("find template: %v", err)
	}
	if path != projectTemplate {
		t.Fatalf("template path = %q, want %q", path, projectTemplate)
	}
}

func TestFindTemplateFallsBackToBuiltin(t *testing.T) {
	t.Parallel()

	path, err := FindTemplate("go-feature", t.TempDir())
	if err != nil {
		t.Fatalf("find builtin template: %v", err)
	}
	if path != "builtin:go-feature" {
		t.Fatalf("builtin path = %q, want %q", path, "builtin:go-feature")
	}
}

func TestListTemplatesDeduplicatesByPriority(t *testing.T) {
	projectRoot := t.TempDir()
	globalHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", globalHome)

	projectDir := filepath.Join(projectRoot, ".praetor", "templates")
	globalDir := filepath.Join(globalHome, "templates")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project templates dir: %v", err)
	}
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("create global templates dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectDir, "shared.json"), []byte(`{"name":"project","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}`), 0o644); err != nil {
		t.Fatalf("write project template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "shared.json"), []byte(`{"name":"global","settings":{"agents":{"executor":{"agent":"codex"},"reviewer":{"agent":"claude"}}},"tasks":[{"id":"TASK-001","title":"Task","acceptance":["done"]}]}`), 0o644); err != nil {
		t.Fatalf("write global template: %v", err)
	}

	templates, err := ListTemplates(projectRoot)
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}

	seen := 0
	for _, tmpl := range templates {
		if tmpl.Name != "shared" {
			continue
		}
		seen++
		if tmpl.Source != "project" {
			t.Fatalf("template source = %q, want project", tmpl.Source)
		}
	}
	if seen != 1 {
		t.Fatalf("expected shared template once, got %d entries", seen)
	}
}

func TestRenderTemplateRequiresAllVariables(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	templatePath := filepath.Join(projectRoot, "missing.json")
	if err := os.WriteFile(templatePath, []byte(`{
  "name": "{{.Name}}",
  "summary": "{{.Summary}}",
  "settings": {
    "agents": {
      "executor": {"agent": "codex"},
      "reviewer": {"agent": "claude"}
    }
  },
  "tasks": [
    {
      "id": "TASK-001",
      "title": "Implement {{.Name}}",
      "description": "{{.Description}}",
      "acceptance": ["done"]
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	if _, err := RenderTemplate(templatePath, map[string]string{
		"Name":    "Auth",
		"Summary": "Implement auth",
	}); err == nil {
		t.Fatal("expected missing variable error")
	}
}

func TestBuiltinTemplatesRenderValidPlans(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"go-feature", "bug-fix", "refactor"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan, err := RenderTemplate("builtin:"+name, map[string]string{
				"Name":        "Auth",
				"Summary":     "Implement authentication",
				"Description": "Implement authentication end-to-end",
			})
			if err != nil {
				t.Fatalf("render template: %v", err)
			}
			if plan.Name == "" {
				t.Fatal("expected non-empty plan name")
			}
			if len(plan.Tasks) == 0 {
				t.Fatal("expected rendered tasks")
			}
		})
	}
}
