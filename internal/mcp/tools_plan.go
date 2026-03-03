package mcp

import (
	"fmt"
	"os"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/state"
	"github.com/opus-domini/praetor/internal/workspace"
)

func registerPlanTools(s *Server) {
	s.tools.register("plan_list", "List all plans for the current project",
		objectSchema(map[string]any{
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, nil),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			statuses, err := store.ListPlanStatuses()
			if err != nil {
				return nil, fmt.Errorf("list plans: %w", err)
			}
			return jsonContent(statuses)
		},
	)

	s.tools.register("plan_show", "Show a plan's full JSON content",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			slug := argString(args, "slug")
			plan, err := domain.LoadPlan(store.PlanFile(slug))
			if err != nil {
				return nil, fmt.Errorf("load plan: %w", err)
			}
			return jsonContent(plan)
		},
	)

	s.tools.register("plan_status", "Get detailed status for a plan",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			slug := argString(args, "slug")
			status, err := store.Status(slug)
			if err != nil {
				return nil, fmt.Errorf("get status: %w", err)
			}
			return jsonContent(status)
		},
	)

	s.tools.register("plan_create", "Create a new plan from a name and optional tasks",
		objectSchema(map[string]any{
			"name":        stringProp("Plan name"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"name"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			name := argString(args, "name")
			slug := domain.Slugify(name)
			slug, err = domain.NextAvailableSlug(store.PlansDir(), slug)
			if err != nil {
				return nil, err
			}
			path, err := domain.NewPlanFile(slug, store.PlansDir())
			if err != nil {
				return nil, err
			}
			return jsonContent(map[string]string{"slug": slug, "path": path})
		},
	)

	s.tools.register("plan_reset", "Reset a plan's runtime state",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			slug := argString(args, "slug")
			plan, err := domain.LoadPlan(store.PlanFile(slug))
			if err != nil {
				return nil, fmt.Errorf("load plan for reset: %w", err)
			}
			if _, err := store.ResetPlanRuntime(slug, plan); err != nil {
				return nil, fmt.Errorf("reset plan: %w", err)
			}
			return textContent("Plan state reset successfully"), nil
		},
	)
}

func resolveStore(defaultDir, overrideDir string) (*state.Store, error) {
	projectDir := defaultDir
	if overrideDir != "" {
		projectDir = overrideDir
	}
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}
	root, err := workspace.ResolveProjectRoot(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	projectHome, err := state.ResolveProjectHome("", root)
	if err != nil {
		return nil, fmt.Errorf("resolve project home: %w", err)
	}
	store := state.NewStore(projectHome)
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}
	return store, nil
}
