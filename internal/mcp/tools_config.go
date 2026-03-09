package mcp

import (
	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/workspace"
)

func registerConfigTools(s *Server) {
	s.tools.register("config_show", "Show resolved configuration with source annotations",
		objectSchema(map[string]any{
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, nil),
		func(args map[string]any) ([]contentBlock, error) {
			projectDir := argString(args, "project_dir")
			if projectDir == "" {
				projectDir = s.projectDir
			}
			if resolved, err := workspace.ResolveProjectRoot(projectDir); err == nil {
				projectDir = resolved
			}
			resolved, _, err := config.LoadResolved(projectDir)
			if err != nil {
				return nil, err
			}
			return jsonContent(resolved)
		},
	)

	s.tools.register("config_set", "Set a configuration value",
		objectSchema(map[string]any{
			"key":     stringProp("Configuration key"),
			"value":   stringProp("Value to set"),
			"project": stringProp("Project path for project-scoped config (optional)"),
		}, []string{"key", "value"}),
		func(args map[string]any) ([]contentBlock, error) {
			key := argString(args, "key")
			value := argString(args, "value")
			project := argString(args, "project")
			configPath := config.Path()
			if err := config.SetValue(configPath, project, key, value); err != nil {
				return nil, err
			}
			return textContent("Configuration updated: " + key + " = " + value), nil
		},
	)
}
