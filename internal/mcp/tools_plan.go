package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

	s.tools.register("plan_create", "Create a new skeleton plan file from a name (use plan_show to inspect, then edit the file at the returned path)",
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

	s.tools.register("plan_run", "Start plan execution in the background. Monitor progress with plan_status and plan_events",
		objectSchema(map[string]any{
			"slug":        stringProp("Plan slug"),
			"executor":    stringProp("Executor agent (default: from plan settings or config)"),
			"reviewer":    stringProp("Reviewer agent (default: from plan settings or config)"),
			"runner":      stringProp("Runner mode: direct, tmux, or pty (default: direct)"),
			"no_review":   boolProp("Skip the review phase"),
			"project_dir": stringProp("Project directory (defaults to current)"),
		}, []string{"slug"}),
		func(args map[string]any) ([]contentBlock, error) {
			slug := strings.TrimSpace(argString(args, "slug"))
			if slug == "" {
				return nil, fmt.Errorf("slug is required")
			}

			// Verify the plan exists before launching.
			store, err := resolveStore(s.projectDir, argString(args, "project_dir"))
			if err != nil {
				return nil, err
			}
			if _, err := domain.LoadPlan(store.PlanFile(slug)); err != nil {
				return nil, fmt.Errorf("load plan: %w", err)
			}

			// Resolve the praetor binary.
			binary, err := os.Executable()
			if err != nil {
				binary = "praetor"
			}

			// Build command arguments.
			cmdArgs := []string{"plan", "run", slug, "--runner", "direct"}
			if executor := strings.TrimSpace(argString(args, "executor")); executor != "" {
				cmdArgs = append(cmdArgs, "--executor", executor)
			}
			if reviewer := strings.TrimSpace(argString(args, "reviewer")); reviewer != "" {
				cmdArgs = append(cmdArgs, "--reviewer", reviewer)
			}
			if runner := strings.TrimSpace(argString(args, "runner")); runner != "" {
				// Override the default "direct" if explicitly provided.
				cmdArgs[4] = runner
			}
			if argBool(args, "no_review") {
				cmdArgs = append(cmdArgs, "--no-review")
			}

			// Resolve workdir for the subprocess.
			workdir := s.projectDir
			if workdir == "" {
				workdir, _ = os.Getwd()
			}

			cmd := exec.Command(binary, cmdArgs...)
			cmd.Dir = workdir
			cmd.Stdin = nil
			cmd.Stdout = nil
			cmd.Stderr = nil

			if err := cmd.Start(); err != nil {
				return nil, fmt.Errorf("start plan run: %w", err)
			}

			// Release the process so it runs independently.
			go func() { _ = cmd.Wait() }()

			return jsonContent(map[string]any{
				"slug":    slug,
				"pid":     cmd.Process.Pid,
				"status":  "started",
				"message": "Plan execution started in background. Use plan_status to monitor progress and plan_events to stream execution events.",
			})
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
