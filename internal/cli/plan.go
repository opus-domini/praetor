package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/orchestration/pipeline"
	localstate "github.com/opus-domini/praetor/internal/state"
	"github.com/opus-domini/praetor/internal/workspace"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Create, manage, and execute task plans",
		Long: `Create, manage, and execute task plans.

Plans are JSON files identified by slug and stored under the praetor home directory.
Use "praetor plan run <slug>" to execute a plan.`,
	}

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newPlanCreateCmd())
	cmd.AddCommand(newPlanEditCmd())
	cmd.AddCommand(newPlanListCmd())
	cmd.AddCommand(newPlanStatusCmd())
	cmd.AddCommand(newPlanShowCmd())
	cmd.AddCommand(newPlanResetCmd())
	cmd.AddCommand(newPlanResumeCmd())
	cmd.AddCommand(newPlanPathCmd())
	cmd.AddCommand(newPlanEvalCmd())
	cmd.AddCommand(newPlanDiagnoseCmd())
	cmd.AddCommand(newPlanExportCmd())
	return cmd
}

func newPlanCreateCmd() *cobra.Command {
	var fromFile string
	var fromTemplate string
	var fromStdin bool
	var dryRun bool
	var noAgent bool
	var noColor bool
	var slugOverride string
	var plannerAgent string
	var plannerModel string
	var plannerTimeout time.Duration
	var templateVars []string
	var force bool

	cmd := &cobra.Command{
		Use:   "create [brief]",
		Short: "Create a plan from text or markdown input",
		Long:  `Create a plan from a textual brief (arg, file, stdin, interactive prompt, or template).`,
		Example: `  praetor plan create "Implement JWT auth and tests"
  praetor plan create --from-file docs/brief.md
  cat brief.md | praetor plan create --stdin
  praetor plan create --from-template go-feature --var Name=auth --var Summary="Implement JWT auth"
  praetor plan create "Refactor billing" --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			out := cmd.OutOrStdout()
			render := NewRenderer(out, noColor)
			projectRoot, err := workspace.ResolveProjectRoot(".")
			if err != nil {
				return err
			}
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			if err := store.Init(); err != nil {
				return err
			}

			cfg, cfgErr := config.Load(projectRoot)
			if cfgErr != nil {
				return cfgErr
			}

			effectivePlanner := strings.TrimSpace(plannerAgent)
			if effectivePlanner == "" {
				effectivePlanner = strings.TrimSpace(cfg.Planner)
			}
			if effectivePlanner == "" {
				effectivePlanner = string(domain.AgentClaude)
			}
			effectivePlanner = string(domain.NormalizeAgent(domain.Agent(effectivePlanner)))
			if _, ok := domain.ValidExecutors[domain.Agent(effectivePlanner)]; !ok {
				return fmt.Errorf("invalid planner agent %q", effectivePlanner)
			}

			effectivePlannerModel := strings.TrimSpace(plannerModel)
			plan := domain.Plan{}
			brief := ""
			if strings.TrimSpace(fromTemplate) != "" {
				if strings.TrimSpace(fromFile) != "" || fromStdin {
					return errors.New("--from-template cannot be combined with --from-file or --stdin")
				}
				if noAgent {
					return errors.New("--from-template cannot be combined with --no-agent")
				}

				templatePath, findErr := config.FindTemplate(fromTemplate, projectRoot)
				if findErr != nil {
					return findErr
				}
				vars, varsErr := parseTemplateVars(templateVars)
				if varsErr != nil {
					return varsErr
				}
				if len(args) > 0 {
					brief = strings.TrimSpace(args[0])
					if brief != "" {
						if strings.TrimSpace(vars["Name"]) == "" {
							vars["Name"] = derivePlanName(brief)
						}
						if strings.TrimSpace(vars["Summary"]) == "" {
							vars["Summary"] = derivePlanSummary(brief)
						}
						if strings.TrimSpace(vars["Description"]) == "" {
							vars["Description"] = brief
						}
					}
				}

				render.Header("Plan Template")
				render.KV("Template:", templatePath)
				render.KV("Planner:", effectivePlanner)
				templated, renderErr := config.RenderTemplate(templatePath, vars)
				if renderErr != nil {
					return renderErr
				}
				plan = *templated
				render.Success("Template rendered successfully.")
			} else if noAgent {
				brief, err = resolvePlanCreateBrief(args, fromFile, fromStdin, cmd.InOrStdin(), out)
				if err != nil {
					return err
				}
				render.Info("Using template mode (--no-agent).")
				plan = buildNoAgentPlanTemplate(brief, effectivePlanner, effectivePlannerModel)
			} else {
				brief, err = resolvePlanCreateBrief(args, fromFile, fromStdin, cmd.InOrStdin(), out)
				if err != nil {
					return err
				}
				plannerModelLabel := strings.TrimSpace(effectivePlannerModel)
				if plannerModelLabel == "" {
					plannerModelLabel = "default"
				}
				render.Header("Plan Generation")
				render.KV("Planner:", effectivePlanner)
				render.KV("Model:", plannerModelLabel)
				render.Info("Brief captured. Starting planner generation...")
				plan, err = createPlanWithAgent(cmd.Context(), planCreateAgentRequest{
					Brief:          brief,
					PlannerAgent:   domain.Agent(effectivePlanner),
					PlannerModel:   effectivePlannerModel,
					PlannerTimeout: plannerTimeout,
					ProjectRoot:    projectRoot,
					Config:         cfg,
					Store:          store,
					Render:         render,
				})
				if err != nil {
					return err
				}
				render.Success("Planner generation completed.")
			}

			author := resolvePlanCreatedBy()
			source := ternary(noAgent, "manual", "agent")
			if strings.TrimSpace(fromTemplate) != "" {
				source = "template:" + strings.TrimSpace(strings.TrimSuffix(fromTemplate, ".json"))
			}
			if err := finalizePlanMetadata(&plan, planFinalizeInput{
				Brief:        brief,
				Source:       source,
				CreatedBy:    author,
				PlannerAgent: domain.Agent(effectivePlanner),
				PlannerModel: effectivePlannerModel,
			}); err != nil {
				return err
			}

			slug := strings.TrimSpace(slugOverride)
			if slug != "" {
				slug = domain.Slugify(slug)
				if !isValidPlanSlug(slug) {
					return fmt.Errorf("invalid slug %q (allowed: lowercase letters, digits, hyphens)", slugOverride)
				}
			} else {
				slug = domain.Slugify(plan.Name)
				slug, err = domain.NextAvailableSlug(store.PlansDir(), slug)
				if err != nil {
					return err
				}
			}

			path := store.PlanFile(slug)
			if _, statErr := os.Stat(path); statErr == nil && !force {
				return fmt.Errorf("plan file already exists: %s (use --force to overwrite)", path)
			}
			if _, statErr := os.Stat(path); statErr == nil && force {
				if status, statusErr := store.Status(slug); statusErr == nil && status.StateFile != "" {
					render.Warn(fmt.Sprintf("Overwriting existing plan with state (done=%d failed=%d active=%d).", status.Done, status.Failed, status.Active))
				}
			}

			if dryRun {
				render.Header("Plan Preview")
				encoded, marshalErr := json.MarshalIndent(plan, "", "  ")
				if marshalErr != nil {
					return fmt.Errorf("encode plan for dry-run: %w", marshalErr)
				}
				encoded = append(encoded, '\n')
				_, err = out.Write(encoded)
				return err
			}

			if err := domain.WriteJSONFile(path, plan); err != nil {
				return err
			}

			absPath, _ := filepath.Abs(path)
			render.Header("Plan Created")
			render.KV("Slug:", slug)
			render.KV("Name:", plan.Name)
			render.KV("Tasks:", fmt.Sprintf("%d", len(plan.Tasks)))
			render.KV("Path:", absPath)
			render.Success("Plan saved successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&fromFile, "from-file", "", "Read plan brief from a text/markdown file")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read plan brief from stdin")
	cmd.Flags().StringVar(&fromTemplate, "from-template", "", "Render a reusable plan template by name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print generated plan JSON without writing file")
	cmd.Flags().BoolVar(&noAgent, "no-agent", false, "Generate a minimal plan template without calling a planner agent")
	cmd.Flags().StringVar(&slugOverride, "slug", "", "Explicit slug override (default: auto-generated from plan name)")
	cmd.Flags().StringVar(&plannerAgent, "planner", "", "Planner agent override (default: config planner or claude)")
	cmd.Flags().StringVar(&plannerModel, "planner-model", "", "Planner model override")
	cmd.Flags().DurationVar(&plannerTimeout, "planner-timeout", 0, "Planner generation timeout (e.g. 3m, 10m); 0 disables timeout")
	cmd.Flags().StringArrayVar(&templateVars, "var", nil, "Template variable in key=value form (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing plan file")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}

func newPlanExportCmd() *cobra.Command {
	var outputDir string
	var force bool

	cmd := &cobra.Command{
		Use:     "export <slug>",
		Short:   "Export a plan bundle with plan, state, summary, and template",
		Example: "  praetor plan export my-plan\n  praetor plan export my-plan --output ./exports/my-plan",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			projectRoot, err := workspace.ResolveProjectRoot(".")
			if err != nil {
				return err
			}
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			if err := store.Init(); err != nil {
				return err
			}

			slug := strings.TrimSpace(args[0])
			planPath := store.PlanFile(slug)
			planBytes, err := os.ReadFile(planPath)
			if err != nil {
				return fmt.Errorf("read plan file: %w", err)
			}
			plan, err := domain.LoadPlan(planPath)
			if err != nil {
				return err
			}
			status, err := store.Status(slug)
			if err != nil {
				return err
			}

			if strings.TrimSpace(outputDir) == "" {
				outputDir = filepath.Join(projectRoot, ".praetor", "exports", slug)
			}
			outputDir, err = filepath.Abs(outputDir)
			if err != nil {
				return fmt.Errorf("resolve export directory: %w", err)
			}
			if _, err := os.Stat(outputDir); err == nil {
				if !force {
					return fmt.Errorf("export directory already exists: %s (use --force to overwrite)", outputDir)
				}
				if removeErr := os.RemoveAll(outputDir); removeErr != nil {
					return fmt.Errorf("remove existing export directory: %w", removeErr)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat export directory: %w", err)
			}
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create export directory: %w", err)
			}

			if err := writeRawFile(filepath.Join(outputDir, "plan.json"), planBytes); err != nil {
				return err
			}
			statePath := store.StateFile(slug)
			if stateBytes, readErr := os.ReadFile(statePath); readErr == nil {
				if err := writeRawFile(filepath.Join(outputDir, "state.json"), stateBytes); err != nil {
					return err
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return fmt.Errorf("read state file: %w", readErr)
			}

			summary, err := buildPlanExportSummary(store, status, plan)
			if err != nil {
				return err
			}
			if err := domain.WriteJSONFile(filepath.Join(outputDir, "summary.json"), summary); err != nil {
				return err
			}

			templatePlan := buildExportTemplate(plan)
			if err := domain.WriteJSONFile(filepath.Join(outputDir, "template.json"), templatePlan); err != nil {
				return err
			}

			render := NewRenderer(cmd.OutOrStdout(), false)
			render.Header("Plan Export")
			render.KV("Slug:", slug)
			render.KV("Output:", outputDir)
			render.KV("Files:", exportedFilesLabel(summary.HasState))
			render.Success("Plan exported successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output", "", "Export directory (default: .praetor/exports/<slug>)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing export directory")
	return cmd
}

func newPlanEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "edit <slug>",
		Short:   "Open a plan in $EDITOR",
		Example: `  praetor plan edit my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			path := store.PlanFile(args[0])
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("plan not found: %s", args[0])
			}
			return openEditor(path)
		},
	}
	return cmd
}

func newPlanShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show <slug>",
		Short:   "Print plan JSON to stdout",
		Example: `  praetor plan show my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			path := store.PlanFile(args[0])
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("plan not found: %s", args[0])
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), string(data))
			return err
		},
	}
	return cmd
}

func newPlanPathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "path <slug>",
		Short:   "Print absolute path of a plan file",
		Example: `  praetor plan path my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), store.PlanFile(args[0]))
			return err
		},
	}
	return cmd
}

func newPlanStatusCmd() *cobra.Command {
	var noColor bool
	var verbose bool

	cmd := &cobra.Command{
		Use:     "status <slug>",
		Short:   "Show execution status for a plan",
		Example: `  praetor plan status my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}
			status, err := store.Status(args[0])
			if err != nil {
				return err
			}
			r := NewRenderer(cmd.OutOrStdout(), noColor)
			if err := printPlanStatus(r, status); err != nil {
				return err
			}
			if !verbose {
				return nil
			}
			snapshot, _, err := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), args[0])
			if err != nil {
				return err
			}
			return printPlanRunSummary(cmd.OutOrStdout(), r, snapshot.Summary)
		},
	}

	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show latest run summary with actor breakdown")
	return cmd
}

func newPlanListCmd() *cobra.Command {
	var noColor bool

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List tracked plans for current project",
		Example: `  praetor plan list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			statuses, err := store.ListPlanStatuses()
			if err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), noColor)
			if len(statuses) == 0 {
				r.Info(fmt.Sprintf("No plans found in %s", store.PlansDir()))
				return nil
			}

			r.Dim(fmt.Sprintf("  %-30s %5s %5s %5s %5s  %s", "Plan", "Done", "Fail", "Left", "Total", "Status"))
			for _, status := range statuses {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-30s %5d %5d %5d %5d  %s\n",
					status.PlanSlug, status.Done, status.Failed, status.Active, status.Total, planStatusLabel(status))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}

func newPlanResetCmd() *cobra.Command {
	var force bool
	var noColor bool

	cmd := &cobra.Command{
		Use:     "reset <slug>",
		Short:   "Clear execution state for a plan",
		Example: `  praetor plan reset my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			slug := args[0]
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			plan, err := domain.LoadPlan(store.PlanFile(slug))
			if err != nil {
				return err
			}

			running, pid := store.IsPlanRunning(slug)
			if running && !force {
				return fmt.Errorf("plan is currently running (pid=%d); use --force to reset anyway", pid)
			}
			removed, err := store.ResetPlanRuntime(slug, plan)
			if err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), noColor)
			if removed == 0 {
				r.Info(fmt.Sprintf("Nothing to reset for: %s", slug))
				return nil
			}
			r.Success(fmt.Sprintf("Reset complete. Removed %d file(s).", removed))
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force reset even if a running lock exists")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}

func newPlanResumeCmd() *cobra.Command {
	var noColor bool

	cmd := &cobra.Command{
		Use:     "resume <slug>",
		Short:   "Restore the latest local snapshot state for a plan",
		Example: `  praetor plan resume my-feature`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			slug := args[0]
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			snapshot, snapshotPath, err := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), slug)
			if err != nil {
				return err
			}
			if strings.TrimSpace(snapshotPath) == "" {
				return fmt.Errorf("no local snapshot found for plan: %s", slug)
			}

			if err := store.Init(); err != nil {
				return err
			}
			if err := store.WriteState(slug, snapshot.State); err != nil {
				return fmt.Errorf("persist resumed state: %w", err)
			}

			r := NewRenderer(cmd.OutOrStdout(), noColor)
			r.Success(fmt.Sprintf("Resumed from: %s", snapshotPath))
			r.KV("State:", store.StateFile(slug))
			r.KV("Progress:", fmt.Sprintf("%d/%d tasks done", snapshot.State.DoneCount(), len(snapshot.State.Tasks)))
			return nil
		},
	}

	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}

func newPlanDiagnoseCmd() *cobra.Command {
	var runID string
	var query string
	var format string
	var baseline string

	cmd := &cobra.Command{
		Use:     "diagnose <slug>",
		Short:   "Inspect runtime diagnostics for a plan run",
		Example: "  praetor plan diagnose my-plan --query errors\n  praetor plan diagnose my-plan --query summary --format json\n  praetor plan diagnose my-plan --query regressions --baseline .local-plans/baseline.json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			slug := strings.TrimSpace(args[0])
			store, err := resolveStore(".")
			if err != nil {
				return err
			}

			runDir, err := resolveDiagnoseRunDir(store, slug, runID)
			if err != nil {
				return err
			}
			eventsPath := filepath.Join(runDir, "events.jsonl")
			performancePath := filepath.Join(runDir, "diagnostics", "performance.jsonl")

			events, err := readJSONLRecords(eventsPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			perf, err := readJSONLRecords(performancePath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}

			query = strings.ToLower(strings.TrimSpace(query))
			if query == "" {
				query = "all"
			}

			switch query {
			case "errors", "stalls", "fallbacks", "all":
				filtered := filterEventsByQuery(events, query)
				if strings.EqualFold(format, "json") {
					return printJSONL(cmd, filtered)
				}
				return printEventsTable(cmd, filtered)
			case "costs":
				if strings.EqualFold(format, "json") {
					return printJSONL(cmd, perf)
				}
				return printCostsTable(cmd, perf)
			case "summary":
				summary := buildDiagnoseSummary(events, perf)
				if strings.EqualFold(format, "json") {
					return printJSONObject(cmd, summary)
				}
				return printSummaryTable(cmd, summary)
			case "regressions":
				if strings.TrimSpace(baseline) == "" {
					return errors.New("query regressions requires --baseline <path>")
				}
				current := buildDiagnoseSummary(events, perf)
				regression, err := buildDiagnoseRegression(strings.TrimSpace(baseline), current)
				if err != nil {
					return err
				}
				if strings.EqualFold(format, "json") {
					return printJSONObject(cmd, regression)
				}
				return printRegressionTable(cmd, regression)
			default:
				return fmt.Errorf("unsupported query %q (allowed: errors, stalls, fallbacks, costs, summary, regressions, all)", query)
			}
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "Inspect a specific run id (default: latest run for the plan)")
	cmd.Flags().StringVar(&query, "query", "all", "Diagnostic query: errors, stalls, fallbacks, costs, summary, regressions, all")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table or json")
	cmd.Flags().StringVar(&baseline, "baseline", "", "Baseline JSON file path for regressions query")
	return cmd
}

func resolveDiagnoseRunDir(store *localstate.Store, slug, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		runDir := filepath.Join(store.RuntimeDir(), runID)
		if _, err := os.Stat(runDir); err != nil {
			return "", fmt.Errorf("run id not found: %s", runID)
		}
		return runDir, nil
	}
	_, snapshotPath, err := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), slug)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(snapshotPath) == "" {
		return "", fmt.Errorf("no runtime snapshots found for plan: %s", slug)
	}
	return filepath.Dir(snapshotPath), nil
}

func readJSONLRecords(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	records := make([]map[string]any, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return records, nil
}

func filterEventsByQuery(events []map[string]any, query string) []map[string]any {
	if query == "all" {
		return events
	}
	filtered := make([]map[string]any, 0, len(events))
	for _, event := range events {
		eventType := strings.ToLower(strings.TrimSpace(eventTypeName(event)))
		switch query {
		case "errors":
			if strings.Contains(eventType, "error") {
				filtered = append(filtered, event)
			}
		case "stalls":
			if eventType == "task_stalled" {
				filtered = append(filtered, event)
			}
		case "fallbacks":
			if eventType == "agent_fallback" {
				filtered = append(filtered, event)
			}
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return stringValue(filtered[i]["timestamp"]) < stringValue(filtered[j]["timestamp"])
	})
	return filtered
}

func eventTypeName(event map[string]any) string {
	if value := stringValue(event["event_type"]); value != "" {
		return value
	}
	return stringValue(event["type"])
}

func printJSONL(cmd *cobra.Command, records []map[string]any) error {
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(encoded)); err != nil {
			return err
		}
	}
	if len(records) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "{}")
		return err
	}
	return nil
}

func printEventsTable(cmd *cobra.Command, events []map[string]any) error {
	if len(events) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No diagnostic events found for this query.")
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-16s %-12s %-10s %s\n", "Timestamp", "Event", "Task", "Phase", "Message"); err != nil {
		return err
	}
	for _, event := range events {
		timestamp := stringValue(event["timestamp"])
		eventType := eventTypeName(event)
		taskID := stringValue(event["task_id"])
		phase := stringValue(event["phase"])
		message := firstNonEmpty(stringValue(event["message"]), stringValue(event["error"]))
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-16s %-12s %-10s %s\n", timestamp, eventType, taskID, phase, message); err != nil {
			return err
		}
	}
	return nil
}

func printCostsTable(cmd *cobra.Command, records []map[string]any) error {
	if len(records) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No cost/performance metrics found.")
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-10s %-8s %-12s %-12s %s\n", "Iteration", "Phase", "Chars", "EstTokens", "Truncated"); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"%-10s %-8s %-12s %-12s %s\n",
			stringValue(record["iteration"]),
			stringValue(record["phase"]),
			stringValue(record["prompt_chars"]),
			stringValue(record["estimated_tokens"]),
			joinAnyArray(record["sections_truncated"]),
		); err != nil {
			return err
		}
	}
	return nil
}

type diagnoseSummary struct {
	EventsTotal          int                `json:"events_total"`
	Errors               int                `json:"errors"`
	Stalls               int                `json:"stalls"`
	Fallbacks            int                `json:"fallbacks"`
	GateFailures         int                `json:"gate_failures"`
	BudgetWarnings       int                `json:"budget_warnings"`
	BudgetExceeded       int                `json:"budget_exceeded"`
	TotalCostUSD         float64            `json:"total_cost_usd"`
	PlanLimitUSD         float64            `json:"plan_limit_usd,omitempty"`
	TaskLimitUSD         float64            `json:"task_limit_usd,omitempty"`
	CostBudgetEnforce    bool               `json:"cost_budget_enforce"`
	AvgPromptChars       float64            `json:"avg_prompt_chars"`
	AvgEstimatedTokens   float64            `json:"avg_estimated_tokens"`
	MostRetriesActor     string             `json:"most_retries_actor,omitempty"`
	MostRetriesCount     int                `json:"most_retries_count,omitempty"`
	MostStallsActor      string             `json:"most_stalls_actor,omitempty"`
	MostStallsCount      int                `json:"most_stalls_count,omitempty"`
	CostByActor          map[string]float64 `json:"cost_by_actor,omitempty"`
	TopFailureCategories map[string]int     `json:"top_failure_categories"`
}

type diagnoseRegressionCheck struct {
	Metric    string  `json:"metric"`
	Baseline  float64 `json:"baseline"`
	Current   float64 `json:"current"`
	Delta     float64 `json:"delta"`
	Threshold float64 `json:"threshold"`
	Status    string  `json:"status"`
	Message   string  `json:"message,omitempty"`
}

type diagnoseRegression struct {
	BaselinePath string                    `json:"baseline_path"`
	Verdict      string                    `json:"verdict"`
	Checks       []diagnoseRegressionCheck `json:"checks"`
}

func buildDiagnoseSummary(events, perf []map[string]any) diagnoseSummary {
	summary := diagnoseSummary{
		EventsTotal:          len(events),
		CostByActor:          make(map[string]float64),
		TopFailureCategories: make(map[string]int),
	}
	retriesByActor := make(map[string]int)
	stallsByActor := make(map[string]int)
	for _, event := range events {
		eventType := strings.ToLower(strings.TrimSpace(eventTypeName(event)))
		actorKey := eventActorKey(event)
		if strings.Contains(eventType, "error") {
			summary.Errors++
			summary.TopFailureCategories[eventType]++
		}
		if eventType == "task_stalled" {
			summary.Stalls++
			if actorKey != "" {
				stallsByActor[actorKey]++
			}
			summary.TopFailureCategories[eventType]++
		}
		if eventType == "agent_fallback" {
			summary.Fallbacks++
		}
		if eventType == "gate_result" {
			action := strings.ToLower(strings.TrimSpace(stringValue(event["action"])))
			if action == "fail" {
				summary.GateFailures++
				summary.TopFailureCategories["gate_fail"]++
			}
		}
		if eventType == "cost_budget_warning" {
			summary.BudgetWarnings++
		}
		if eventType == "cost_budget_exceeded" {
			summary.BudgetExceeded++
			summary.TopFailureCategories["cost_budget_exceeded"]++
		}
		summary.TotalCostUSD += anyFloat(event["cost_usd"])
		if actorKey != "" && anyFloat(event["cost_usd"]) > 0 {
			summary.CostByActor[actorKey] += anyFloat(event["cost_usd"])
		}
		if eventType == "task_failed" {
			if data, ok := event["data"].(map[string]any); ok {
				if retry, ok := data["retry"].(bool); ok && retry && actorKey != "" {
					retriesByActor[actorKey]++
				}
			}
		}
		if data, ok := event["data"].(map[string]any); ok {
			if limit := anyInt64(data["plan_limit_cents"]); limit > 0 {
				summary.PlanLimitUSD = float64(limit) / 100
			}
			if limit := anyInt64(data["task_limit_cents"]); limit > 0 {
				summary.TaskLimitUSD = float64(limit) / 100
			}
			if limit := anyInt64(data["limit_micros"]); limit > 0 {
				scope := strings.ToLower(strings.TrimSpace(stringValue(data["scope"])))
				if scope == "task" {
					summary.TaskLimitUSD = float64(limit) / 1_000_000
				} else {
					summary.PlanLimitUSD = float64(limit) / 1_000_000
				}
			}
			if enforce, ok := data["cost_budget_enforce"].(bool); ok {
				summary.CostBudgetEnforce = enforce
			}
		}
	}
	summary.MostRetriesActor, summary.MostRetriesCount = maxActorCount(retriesByActor)
	summary.MostStallsActor, summary.MostStallsCount = maxActorCount(stallsByActor)
	if len(perf) > 0 {
		var promptChars float64
		var tokens float64
		for _, item := range perf {
			promptChars += anyFloat(item["prompt_chars"])
			tokens += anyFloat(item["estimated_tokens"])
		}
		summary.AvgPromptChars = promptChars / float64(len(perf))
		summary.AvgEstimatedTokens = tokens / float64(len(perf))
	}
	return summary
}

func buildDiagnoseRegression(baselinePath string, current diagnoseSummary) (diagnoseRegression, error) {
	baselinePath = strings.TrimSpace(baselinePath)
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return diagnoseRegression{}, fmt.Errorf("read baseline file: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return diagnoseRegression{}, fmt.Errorf("decode baseline file: %w", err)
	}

	baselineMap := payload
	if summaryValue, ok := payload["summary"]; ok {
		if nested, ok := summaryValue.(map[string]any); ok {
			baselineMap = nested
		}
	}

	baseline := diagnoseSummary{
		Errors:             anyInt(baselineMap["errors"]),
		Stalls:             anyInt(baselineMap["stalls"]),
		GateFailures:       anyInt(baselineMap["gate_failures"]),
		TotalCostUSD:       anyFloat(baselineMap["total_cost_usd"]),
		AvgPromptChars:     anyFloat(baselineMap["avg_prompt_chars"]),
		AvgEstimatedTokens: anyFloat(baselineMap["avg_estimated_tokens"]),
	}

	result := diagnoseRegression{
		BaselinePath: baselinePath,
		Verdict:      "pass",
		Checks: []diagnoseRegressionCheck{
			makeDiagnoseIncreaseCheck("errors", float64(baseline.Errors), float64(current.Errors), 0),
			makeDiagnoseIncreaseCheck("stalls", float64(baseline.Stalls), float64(current.Stalls), 0),
			makeDiagnoseIncreaseCheck("gate_failures", float64(baseline.GateFailures), float64(current.GateFailures), 0),
			makeDiagnoseIncreaseCheck("avg_prompt_chars", baseline.AvgPromptChars, current.AvgPromptChars, baseline.AvgPromptChars*0.20),
			makeDiagnoseIncreaseCheck("avg_estimated_tokens", baseline.AvgEstimatedTokens, current.AvgEstimatedTokens, baseline.AvgEstimatedTokens*0.20),
		},
	}

	costCheck := diagnoseRegressionCheck{
		Metric:    "total_cost_usd_pct",
		Baseline:  baseline.TotalCostUSD,
		Current:   current.TotalCostUSD,
		Threshold: 20,
		Status:    "pass",
	}
	if baseline.TotalCostUSD > 0 {
		costCheck.Delta = ((current.TotalCostUSD - baseline.TotalCostUSD) / baseline.TotalCostUSD) * 100
		if costCheck.Delta > costCheck.Threshold {
			costCheck.Status = "fail"
			costCheck.Message = fmt.Sprintf("cost increase %.2f%% exceeds %.2f%%", costCheck.Delta, costCheck.Threshold)
		}
	} else if current.TotalCostUSD > 0 {
		costCheck.Status = "warn"
		costCheck.Message = "baseline total_cost_usd is zero; percentage comparison skipped"
	}
	result.Checks = append(result.Checks, costCheck)

	hasFail := false
	hasWarn := false
	for _, check := range result.Checks {
		status := strings.ToLower(strings.TrimSpace(check.Status))
		if status == "fail" {
			hasFail = true
		}
		if status == "warn" {
			hasWarn = true
		}
	}
	if hasFail {
		result.Verdict = "fail"
	} else if hasWarn {
		result.Verdict = "warn"
	}
	return result, nil
}

func makeDiagnoseIncreaseCheck(metric string, baseline, current, threshold float64) diagnoseRegressionCheck {
	check := diagnoseRegressionCheck{
		Metric:    metric,
		Baseline:  baseline,
		Current:   current,
		Delta:     current - baseline,
		Threshold: threshold,
		Status:    "pass",
	}
	if check.Delta > check.Threshold {
		check.Status = "fail"
		check.Message = fmt.Sprintf("delta %.4f exceeds threshold %.4f", check.Delta, check.Threshold)
	}
	return check
}

func printJSONObject(cmd *cobra.Command, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = cmd.OutOrStdout().Write(encoded)
	return err
}

func printSummaryTable(cmd *cobra.Command, summary diagnoseSummary) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Events: %d\n", summary.EventsTotal); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Errors: %d  Stalls: %d  Fallbacks: %d  Gate failures: %d\n", summary.Errors, summary.Stalls, summary.Fallbacks, summary.GateFailures); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Budget warnings: %d  Budget exceeded: %d\n", summary.BudgetWarnings, summary.BudgetExceeded); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Total cost: $%.4f  Avg prompt chars: %.2f  Avg est tokens: %.2f\n", summary.TotalCostUSD, summary.AvgPromptChars, summary.AvgEstimatedTokens); err != nil {
		return err
	}
	if summary.MostRetriesActor != "" {
		if _, err := fmt.Fprintf(out, "Most retries: %s (%d)\n", summary.MostRetriesActor, summary.MostRetriesCount); err != nil {
			return err
		}
	}
	if summary.MostStallsActor != "" {
		if _, err := fmt.Fprintf(out, "Most stalls: %s (%d)\n", summary.MostStallsActor, summary.MostStallsCount); err != nil {
			return err
		}
	}
	if summary.PlanLimitUSD > 0 {
		mode := "warn-only"
		if summary.CostBudgetEnforce {
			mode = "enforce"
		}
		if _, err := fmt.Fprintf(out, "Plan budget: $%.2f (%s)\n", summary.PlanLimitUSD, mode); err != nil {
			return err
		}
	}
	if summary.TaskLimitUSD > 0 {
		if _, err := fmt.Fprintf(out, "Task budget: $%.2f\n", summary.TaskLimitUSD); err != nil {
			return err
		}
	}
	if len(summary.TopFailureCategories) > 0 {
		if _, err := fmt.Fprintln(out, "Top failure categories:"); err != nil {
			return err
		}
		keys := make([]string, 0, len(summary.TopFailureCategories))
		for key := range summary.TopFailureCategories {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, err := fmt.Fprintf(out, "- %s: %d\n", key, summary.TopFailureCategories[key]); err != nil {
				return err
			}
		}
	}
	if len(summary.CostByActor) > 0 {
		keys := make([]string, 0, len(summary.CostByActor))
		for key := range summary.CostByActor {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := fmt.Fprintln(out, "Cost by actor:"); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(out, "- %s: $%.4f\n", key, summary.CostByActor[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func printRegressionTable(cmd *cobra.Command, regression diagnoseRegression) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Baseline: %s\n", regression.BaselinePath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Verdict: %s\n", strings.ToUpper(strings.TrimSpace(regression.Verdict))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%-24s %-10s %-10s %-10s %-10s %s\n", "Metric", "Baseline", "Current", "Delta", "Threshold", "Status"); err != nil {
		return err
	}
	for _, check := range regression.Checks {
		if _, err := fmt.Fprintf(out, "%-24s %-10.4f %-10.4f %-10.4f %-10.4f %s\n",
			check.Metric,
			check.Baseline,
			check.Current,
			check.Delta,
			check.Threshold,
			strings.ToUpper(strings.TrimSpace(check.Status)),
		); err != nil {
			return err
		}
		if strings.TrimSpace(check.Message) != "" {
			if _, err := fmt.Fprintf(out, "  %s\n", strings.TrimSpace(check.Message)); err != nil {
				return err
			}
		}
	}
	return nil
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%.2f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return ""
	}
}

func joinAnyArray(value any) string {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if s := stringValue(item); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ",")
}

func anyFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func anyInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func anyInt64(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func eventActorKey(event map[string]any) string {
	if actor, ok := event["actor"].(map[string]any); ok {
		role := strings.TrimSpace(stringValue(actor["role"]))
		agent := strings.TrimSpace(stringValue(actor["agent"]))
		if role != "" || agent != "" {
			if role == "" {
				role = "unknown"
			}
			if agent == "" {
				agent = "unknown"
			}
			return role + ":" + agent
		}
	}
	role := strings.TrimSpace(stringValue(event["role"]))
	agent := strings.TrimSpace(stringValue(event["agent"]))
	if role == "" && agent == "" {
		return ""
	}
	if role == "" {
		role = "unknown"
	}
	if agent == "" {
		agent = "unknown"
	}
	return role + ":" + agent
}

func maxActorCount(values map[string]int) (string, int) {
	bestKey := ""
	bestValue := 0
	for key, value := range values {
		if value > bestValue || (value == bestValue && key < bestKey) {
			bestKey = key
			bestValue = value
		}
	}
	return bestKey, bestValue
}

func resolveStore(projectDir string) (*localstate.Store, error) {
	root, err := localstate.ResolveProjectHome("", projectDir)
	if err != nil {
		return nil, err
	}
	return localstate.NewStore(root), nil
}

type planCreateAgentRequest struct {
	Brief          string
	PlannerAgent   domain.Agent
	PlannerModel   string
	PlannerTimeout time.Duration
	ProjectRoot    string
	Config         config.Config
	Store          *localstate.Store
	Render         *Renderer
}

const plannerCreateMaxAttempts = 2

type planFinalizeInput struct {
	Brief        string
	Source       string
	CreatedBy    string
	PlannerAgent domain.Agent
	PlannerModel string
}

type planExportSummary struct {
	PlanSlug        string                 `json:"plan_slug"`
	PlanName        string                 `json:"plan_name"`
	ExportedAt      string                 `json:"exported_at"`
	UpdatedAt       string                 `json:"updated_at,omitempty"`
	Outcome         domain.RunOutcome      `json:"outcome,omitempty"`
	Total           int                    `json:"total"`
	Done            int                    `json:"done"`
	Failed          int                    `json:"failed"`
	Active          int                    `json:"active"`
	Running         bool                   `json:"running"`
	HasState        bool                   `json:"has_state"`
	LatestRunID     string                 `json:"latest_run_id,omitempty"`
	ExecutionPolicy domain.ExecutionPolicy `json:"execution_policy,omitempty"`
	TotalCostUSD    float64                `json:"total_cost_usd"`
	PlanLimitUSD    float64                `json:"plan_limit_usd,omitempty"`
	TaskLimitUSD    float64                `json:"task_limit_usd,omitempty"`
	RunSummary      domain.RunSummary      `json:"run_summary,omitempty"`
}

var planSlugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func resolvePlanCreateBrief(args []string, fromFile string, fromStdin bool, stdin io.Reader, out io.Writer) (string, error) {
	argBrief := ""
	if len(args) > 0 {
		argBrief = strings.TrimSpace(args[0])
	}
	fileSource := strings.TrimSpace(fromFile) != ""

	sources := 0
	if argBrief != "" {
		sources++
	}
	if fileSource {
		sources++
	}
	if fromStdin {
		sources++
	}
	if sources > 1 {
		return "", errors.New("provide exactly one brief source: argument, --from-file, or --stdin")
	}
	if sources == 0 {
		if _, err := fmt.Fprintln(out, "Type or paste the plan brief below."); err != nil {
			return "", fmt.Errorf("write interactive brief prompt: %w", err)
		}
		if _, err := fmt.Fprintln(out, "Finish with an empty line twice (Enter, Enter) or Ctrl+D (Linux/macOS) / Ctrl+Z then Enter (Windows):"); err != nil {
			return "", fmt.Errorf("write interactive brief prompt: %w", err)
		}
		brief, err := readInteractiveBrief(stdin)
		if err != nil {
			return "", err
		}
		if brief == "" {
			return "", errors.New("plan brief is empty")
		}
		return brief, nil
	}

	if argBrief != "" {
		return argBrief, nil
	}
	if fileSource {
		data, err := os.ReadFile(strings.TrimSpace(fromFile))
		if err != nil {
			return "", fmt.Errorf("read brief file: %w", err)
		}
		brief := strings.TrimSpace(string(data))
		if brief == "" {
			return "", errors.New("brief file is empty")
		}
		return brief, nil
	}
	if fromStdin {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin brief: %w", err)
		}
		brief := strings.TrimSpace(string(data))
		if brief == "" {
			return "", errors.New("stdin brief is empty")
		}
		return brief, nil
	}
	return "", errors.New("unable to resolve brief source")
}

func readInteractiveBrief(stdin io.Reader) (string, error) {
	reader := bufio.NewReader(stdin)
	lines := make([]string, 0, 16)
	blankStreak := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read interactive brief: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			blankStreak++
			if blankStreak >= 2 {
				break
			}
			if !errors.Is(err, io.EOF) {
				lines = append(lines, "")
			}
		} else {
			blankStreak = 0
			lines = append(lines, line)
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func createPlanWithAgent(ctx context.Context, req planCreateAgentRequest) (domain.Plan, error) {
	started := time.Now()
	runtime, err := pipeline.BuildAgentRuntimeWithDeps(domain.RunnerOptions{
		RunnerMode:       domain.RunnerDirect,
		CodexBin:         firstNonEmpty(req.Config.CodexBin, "codex"),
		ClaudeBin:        firstNonEmpty(req.Config.ClaudeBin, "claude"),
		CopilotBin:       firstNonEmpty(req.Config.CopilotBin, "copilot"),
		GeminiBin:        firstNonEmpty(req.Config.GeminiBin, "gemini"),
		KimiBin:          firstNonEmpty(req.Config.KimiBin, "kimi"),
		OpenCodeBin:      firstNonEmpty(req.Config.OpenCodeBin, "opencode"),
		OpenRouterURL:    firstNonEmpty(req.Config.OpenRouterURL, "https://openrouter.ai/api/v1"),
		OpenRouterModel:  firstNonEmpty(req.Config.OpenRouterModel, "openai/gpt-4o-mini"),
		OpenRouterKeyEnv: firstNonEmpty(req.Config.OpenRouterKeyEnv, "OPENROUTER_API_KEY"),
		OllamaURL:        firstNonEmpty(req.Config.OllamaURL, "http://127.0.0.1:11434"),
		OllamaModel:      firstNonEmpty(req.Config.OllamaModel, "llama3"),
		LMStudioURL:      firstNonEmpty(req.Config.LMStudioURL, "http://localhost:1234"),
		LMStudioModel:    req.Config.LMStudioModel,
		LMStudioKeyEnv:   firstNonEmpty(req.Config.LMStudioKeyEnv, "LMSTUDIO_API_KEY"),
	}, pipeline.RuntimeDeps{})
	if err != nil {
		return domain.Plan{}, fmt.Errorf("build planner runtime: %w", err)
	}

	planner, err := pipeline.NewCognitiveAgent(req.PlannerAgent, runtime)
	if err != nil {
		return domain.Plan{}, fmt.Errorf("init planner agent: %w", err)
	}

	projectContext := ""
	if manifest, manifestErr := workspace.ReadManifest(req.ProjectRoot); manifestErr == nil {
		projectContext = manifest.Context
	}
	plannerCtx := ctx
	var cancel context.CancelFunc
	if req.PlannerTimeout > 0 {
		plannerCtx, cancel = context.WithTimeout(ctx, req.PlannerTimeout)
		defer cancel()
	}

	planReq := pipeline.PlanRequest{
		Objective:      strings.TrimSpace(req.Brief),
		ProjectContext: projectContext,
		Workdir:        req.ProjectRoot,
		Model:          strings.TrimSpace(req.PlannerModel),
		CodexBin:       firstNonEmpty(req.Config.CodexBin, "codex"),
		ClaudeBin:      firstNonEmpty(req.Config.ClaudeBin, "claude"),
	}
	plan, firstElapsed, err := runPlannerWithProgress(plannerCtx, planner, planReq, req.Render, 1, plannerCreateMaxAttempts)
	if err == nil {
		if req.Render != nil {
			req.Render.Info(plannerTotalDurationMessage(time.Since(started), 1))
		}
		return plan, nil
	}
	if timeoutErr := plannerTimeoutError(err, req.PlannerTimeout); timeoutErr != nil {
		return domain.Plan{}, timeoutErr
	}

	var outputErr *pipeline.PlannerOutputError
	if errors.As(err, &outputErr) && strings.EqualFold(strings.TrimSpace(outputErr.Class), "recoverable_format") {
		logPath, logErr := writePlannerFailureLog(req.Store.LogsDir(), outputErr.RawOutput)
		if req.Render != nil {
			req.Render.Warn(plannerRetryWarningMessage(1, plannerCreateMaxAttempts, firstElapsed, logPath))
			if logErr != nil {
				req.Render.Warn(fmt.Sprintf("Planner %s raw output could not be logged: %v", plannerAttemptTag(1, plannerCreateMaxAttempts), logErr))
			}
		}
		retryReq := planReq
		retryReq.Objective = buildPlannerRecoveryObjective(planReq.Objective, outputErr.RawOutput)
		plan, _, retryErr := runPlannerWithProgress(plannerCtx, planner, retryReq, req.Render, 2, plannerCreateMaxAttempts)
		if retryErr == nil {
			if req.Render != nil {
				req.Render.Info(fmt.Sprintf("Planner recovered on %s and returned a valid plan.", plannerAttemptTag(2, plannerCreateMaxAttempts)))
				req.Render.Info(plannerTotalDurationMessage(time.Since(started), 2))
			}
			return plan, nil
		}
		err = retryErr
		if timeoutErr := plannerTimeoutError(err, req.PlannerTimeout); timeoutErr != nil {
			return domain.Plan{}, timeoutErr
		}
		if errors.As(retryErr, &outputErr) && strings.TrimSpace(outputErr.RawOutput) != "" {
			preview := plannerOutputPreview(outputErr.RawOutput)
			logPath, logErr := writePlannerFailureLog(req.Store.LogsDir(), outputErr.RawOutput)
			if logErr == nil {
				parseClass := strings.TrimSpace(outputErr.Class)
				if parseClass != "" {
					return domain.Plan{}, fmt.Errorf("planner generated invalid plan (%s, output logged at %s): %w. Output preview: %q. Retry with --dry-run or --no-agent", parseClass, logPath, err, preview)
				}
				return domain.Plan{}, fmt.Errorf("planner generated invalid plan (output logged at %s): %w. Output preview: %q. Retry with --dry-run or --no-agent", logPath, err, preview)
			}
		}
		return domain.Plan{}, fmt.Errorf("planner failed after retry: %w. Retry with --dry-run or --no-agent", err)
	}

	if errors.As(err, &outputErr) && strings.TrimSpace(outputErr.RawOutput) != "" {
		preview := plannerOutputPreview(outputErr.RawOutput)
		logPath, logErr := writePlannerFailureLog(req.Store.LogsDir(), outputErr.RawOutput)
		if logErr == nil {
			parseClass := strings.TrimSpace(outputErr.Class)
			if parseClass != "" {
				return domain.Plan{}, fmt.Errorf("planner generated invalid plan (%s, output logged at %s): %w. Output preview: %q. Retry with --dry-run or --no-agent", parseClass, logPath, err, preview)
			}
			return domain.Plan{}, fmt.Errorf("planner generated invalid plan (output logged at %s): %w. Output preview: %q. Retry with --dry-run or --no-agent", logPath, err, preview)
		}
	}
	return domain.Plan{}, fmt.Errorf("planner failed: %w. Retry with --dry-run or --no-agent", err)
}

func runPlannerWithProgress(ctx context.Context, planner pipeline.CognitiveAgent, req pipeline.PlanRequest, render *Renderer, attempt, totalAttempts int) (domain.Plan, time.Duration, error) {
	started := time.Now()
	elapsed := func() time.Duration {
		return time.Since(started)
	}
	if planner == nil {
		return domain.Plan{}, elapsed(), errors.New("planner is required")
	}
	if render == nil {
		plan, err := planner.Plan(ctx, req)
		return plan, elapsed(), err
	}

	render.Info(plannerStartMessage(attempt, totalAttempts))
	done := make(chan struct{})
	firstUpdate := time.NewTimer(20 * time.Second)
	ticker := time.NewTicker(40 * time.Second)
	defer firstUpdate.Stop()
	defer ticker.Stop()

	go func() {
		firstHint := true
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-firstUpdate.C:
				elapsed := elapsed()
				if firstHint {
					render.Info(plannerProgressMessage(attempt, totalAttempts, elapsed, true))
					firstHint = false
					continue
				}
				render.Info(plannerProgressMessage(attempt, totalAttempts, elapsed, false))
			case <-ticker.C:
				render.Info(plannerProgressMessage(attempt, totalAttempts, elapsed(), false))
			}
		}
	}()

	plan, err := planner.Plan(ctx, req)
	close(done)
	return plan, elapsed(), err
}

func plannerTimeoutError(err error, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("planner timed out after %s; retry with --planner-timeout <duration>, simplify the brief, or use --no-agent", timeout)
	}
	return nil
}

func plannerAttemptTag(attempt, totalAttempts int) string {
	if totalAttempts <= 1 {
		return fmt.Sprintf("attempt %d", attempt)
	}
	return fmt.Sprintf("attempt %d/%d", attempt, totalAttempts)
}

func plannerStartMessage(attempt, totalAttempts int) string {
	return fmt.Sprintf("Starting planner %s...", plannerAttemptTag(attempt, totalAttempts))
}

func plannerProgressMessage(attempt, totalAttempts int, elapsed time.Duration, allowCancel bool) string {
	message := fmt.Sprintf("Planner %s still running... elapsed %s", plannerAttemptTag(attempt, totalAttempts), plannerDurationString(elapsed))
	if allowCancel {
		message += " (Ctrl+C to cancel)"
	}
	return message
}

func plannerRetryWarningMessage(attempt, totalAttempts int, elapsed time.Duration, logPath string) string {
	message := fmt.Sprintf("Planner %s returned output without a valid JSON object after %s.", plannerAttemptTag(attempt, totalAttempts), plannerDurationString(elapsed))
	if strings.TrimSpace(logPath) != "" {
		message += fmt.Sprintf(" Raw output logged at %s.", logPath)
	}
	message += " Retrying once with strict JSON instructions..."
	return message
}

func plannerTotalDurationMessage(totalElapsed time.Duration, attemptsUsed int) string {
	label := "attempt"
	if attemptsUsed != 1 {
		label = "attempts"
	}
	return fmt.Sprintf("Planner total duration: %s across %d %s.", plannerDurationString(totalElapsed), attemptsUsed, label)
}

func plannerDurationString(elapsed time.Duration) string {
	elapsed = elapsed.Round(time.Second)
	if elapsed < 0 {
		return "0s"
	}
	return elapsed.String()
}

func buildPlannerRecoveryObjective(objective, previousOutput string) string {
	objective = strings.TrimSpace(objective)
	previousOutput = strings.TrimSpace(previousOutput)
	if len(previousOutput) > 1500 {
		previousOutput = previousOutput[:1500] + "\n...[truncated]"
	}

	return strings.TrimSpace(fmt.Sprintf(`The previous planner response was invalid because it did not return a valid JSON plan.

STRICT RULES:
- Return ONE JSON object only.
- Do not execute actions.
- Do not claim files were created.
- Do not include markdown fences or explanations.

Original objective:
%s

Previous invalid response:
%s`, objective, previousOutput))
}

func plannerOutputPreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	line := raw
	if idx := strings.IndexRune(raw, '\n'); idx >= 0 {
		line = raw[:idx]
	}
	line = strings.TrimSpace(line)
	if len(line) > 180 {
		line = line[:180] + "..."
	}
	return line
}

func writePlannerFailureLog(logRoot, output string) (string, error) {
	ts := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join(logRoot, "planner-failures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.log", ts))
	content := strings.TrimSpace(output)
	if content == "" {
		content = "<empty output>"
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func parseTemplateVars(values []string) (map[string]string, error) {
	vars := make(map[string]string, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		key, value, found := strings.Cut(raw, "=")
		if !found {
			return nil, fmt.Errorf("invalid template variable %q (expected key=value)", raw)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid template variable %q (empty key)", raw)
		}
		vars[key] = strings.TrimSpace(value)
	}
	return vars, nil
}

func buildNoAgentPlanTemplate(brief, plannerAgent, plannerModel string) domain.Plan {
	name := derivePlanName(brief)
	summary := derivePlanSummary(brief)
	return domain.Plan{
		Name:    name,
		Summary: summary,
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Planner:  domain.PlanAgentConfig{Agent: domain.Agent(plannerAgent), Model: strings.TrimSpace(plannerModel)},
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentClaude},
			},
		},
		Tasks: []domain.Task{
			{
				ID:          "TASK-001",
				Title:       "Implement initial scope",
				Description: summary,
				Acceptance: []string{
					"Implementation complete for the requested scope",
					"Relevant tests or checks pass",
				},
			},
		},
	}
}

func finalizePlanMetadata(plan *domain.Plan, in planFinalizeInput) error {
	if plan == nil {
		return errors.New("plan is nil")
	}
	plan.Name = strings.TrimSpace(plan.Name)
	if plan.Name == "" {
		plan.Name = derivePlanName(in.Brief)
	}
	if strings.TrimSpace(plan.Summary) == "" {
		plan.Summary = derivePlanSummary(in.Brief)
	}
	if strings.TrimSpace(plan.Meta.Source) == "" {
		plan.Meta.Source = strings.TrimSpace(in.Source)
	}
	if strings.TrimSpace(plan.Meta.CreatedAt) == "" {
		plan.Meta.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(plan.Meta.CreatedBy) == "" {
		plan.Meta.CreatedBy = strings.TrimSpace(in.CreatedBy)
	}
	if strings.TrimSpace(plan.Meta.Generator.Name) == "" {
		plan.Meta.Generator.Name = "praetor"
	}
	if strings.TrimSpace(plan.Meta.Generator.Version) == "" {
		plan.Meta.Generator.Version = firstNonEmpty(strings.TrimSpace(os.Getenv("PRAETOR_VERSION")), "dev")
	}
	if strings.TrimSpace(plan.Meta.Generator.PromptHash) == "" && strings.TrimSpace(in.Brief) != "" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(in.Brief)))
		plan.Meta.Generator.PromptHash = "sha256:" + hex.EncodeToString(sum[:])
	}

	if planner := domain.NormalizeAgent(plan.Settings.Agents.Planner.Agent); planner == "" {
		plan.Settings.Agents.Planner.Agent = domain.NormalizeAgent(in.PlannerAgent)
	}
	if strings.TrimSpace(plan.Settings.Agents.Planner.Model) == "" {
		plan.Settings.Agents.Planner.Model = strings.TrimSpace(in.PlannerModel)
	}
	if executor := domain.NormalizeAgent(plan.Settings.Agents.Executor.Agent); executor == "" {
		plan.Settings.Agents.Executor.Agent = domain.AgentCodex
	}
	if reviewer := domain.NormalizeAgent(plan.Settings.Agents.Reviewer.Agent); reviewer == "" {
		plan.Settings.Agents.Reviewer.Agent = domain.AgentClaude
	}

	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		task.ID = strings.TrimSpace(task.ID)
		if task.ID == "" {
			task.ID = fmt.Sprintf("TASK-%03d", i+1)
		}
		task.Title = strings.TrimSpace(task.Title)
		if task.Title == "" {
			task.Title = fmt.Sprintf("Task %d", i+1)
		}
		task.Description = strings.TrimSpace(task.Description)
		task.DependsOn = domain.NormalizedDependsOn(task.DependsOn)
		acceptance := make([]string, 0, len(task.Acceptance))
		for _, item := range task.Acceptance {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			acceptance = append(acceptance, item)
		}
		if len(acceptance) == 0 {
			acceptance = []string{fmt.Sprintf("%s is complete and validated", task.Title)}
		}
		task.Acceptance = acceptance
	}

	return domain.ValidatePlan(*plan)
}

func derivePlanName(brief string) string {
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return "Generated plan"
	}
	lines := strings.Split(brief, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" {
			continue
		}
		if len(line) > 80 {
			return strings.TrimSpace(line[:80])
		}
		return line
	}
	return "Generated plan"
}

func derivePlanSummary(brief string) string {
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return "Plan generated by praetor."
	}
	if len(brief) <= 220 {
		return brief
	}
	return strings.TrimSpace(brief[:220]) + "..."
}

func resolvePlanCreatedBy() string {
	if user := strings.TrimSpace(os.Getenv("USER")); user != "" {
		return user
	}
	cmd := exec.Command("git", "config", "user.name")
	out, err := cmd.Output()
	if err == nil {
		if value := strings.TrimSpace(string(out)); value != "" {
			return value
		}
	}
	return ""
}

func buildPlanExportSummary(store *localstate.Store, status domain.PlanStatus, plan domain.Plan) (planExportSummary, error) {
	summary := planExportSummary{
		PlanSlug:        status.PlanSlug,
		PlanName:        plan.Name,
		ExportedAt:      time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:       status.UpdatedAt,
		Outcome:         status.Outcome,
		Total:           status.Total,
		Done:            status.Done,
		Failed:          status.Failed,
		Active:          status.Active,
		Running:         status.Running,
		HasState:        status.StateFile != "",
		ExecutionPolicy: status.ExecutionPolicy,
		TotalCostUSD:    formatUSDFloat(status.TotalCostMicros),
		PlanLimitUSD:    formatUSDBudgetFloat(status.ExecutionPolicy.Cost.PlanLimitCents),
		TaskLimitUSD:    formatUSDBudgetFloat(status.ExecutionPolicy.Cost.TaskLimitCents),
	}

	snapshot, path, err := localstate.LoadLatestLocalSnapshot(store.RuntimeDir(), status.PlanSlug)
	if err != nil {
		return planExportSummary{}, fmt.Errorf("load latest snapshot: %w", err)
	}
	if path != "" {
		summary.LatestRunID = snapshot.RunID
		summary.RunSummary = snapshot.Summary
		if summary.UpdatedAt == "" {
			summary.UpdatedAt = snapshot.Timestamp
		}
		if summary.Outcome == "" {
			summary.Outcome = snapshot.Outcome
		}
	}
	if summary.RunSummary.TotalCostUSD == 0 && summary.TotalCostUSD > 0 {
		summary.RunSummary.TotalCostUSD = summary.TotalCostUSD
	}
	if summary.RunSummary.TasksDone == 0 {
		summary.RunSummary.TasksDone = summary.Done
	}
	if summary.RunSummary.TasksFailed == 0 {
		summary.RunSummary.TasksFailed = summary.Failed
	}
	return summary, nil
}

func buildExportTemplate(plan domain.Plan) domain.Plan {
	templatePlan := plan
	templatePlan.Meta = domain.PlanMeta{}
	nameValue := strings.TrimSpace(plan.Name)
	summaryValue := strings.TrimSpace(plan.Summary)

	if nameValue != "" {
		templatePlan.Name = "{{.Name}}"
	}
	if summaryValue != "" {
		templatePlan.Summary = "{{.Summary}}"
	}
	for i := range templatePlan.Tasks {
		templatePlan.Tasks[i].Title = replaceTemplatePlaceholder(templatePlan.Tasks[i].Title, nameValue, "{{.Name}}")
		templatePlan.Tasks[i].Title = replaceTemplatePlaceholder(templatePlan.Tasks[i].Title, summaryValue, "{{.Summary}}")
		description := templatePlan.Tasks[i].Description
		if strings.TrimSpace(description) == summaryValue && summaryValue != "" {
			templatePlan.Tasks[i].Description = "{{.Description}}"
			continue
		}
		description = replaceTemplatePlaceholder(description, nameValue, "{{.Name}}")
		description = replaceTemplatePlaceholder(description, summaryValue, "{{.Summary}}")
		templatePlan.Tasks[i].Description = description
	}
	return templatePlan
}

func replaceTemplatePlaceholder(value, original, placeholder string) string {
	original = strings.TrimSpace(original)
	if original == "" {
		return value
	}
	return strings.ReplaceAll(value, original, placeholder)
}

func formatUSDFloat(micros int64) float64 {
	if micros <= 0 {
		return 0
	}
	return float64(micros) / 1_000_000
}

func formatUSDBudgetFloat(cents int64) float64 {
	if cents <= 0 {
		return 0
	}
	return float64(cents) / 100
}

func exportedFilesLabel(hasState bool) string {
	if hasState {
		return "plan.json, state.json, summary.json, template.json"
	}
	return "plan.json, summary.json, template.json"
}

func writeRawFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write tmp file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename tmp file %s: %w", path, err)
	}
	return nil
}

func isValidPlanSlug(slug string) bool {
	return planSlugRegex.MatchString(strings.TrimSpace(slug))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ternary(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

func openEditor(path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return nil
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printPlanStatus(r *Renderer, status domain.PlanStatus) error {
	r.KV("Plan:", status.PlanSlug)
	if status.StateFile == "" {
		r.KV("State:", "not started")
		r.KV("Tasks:", fmt.Sprintf("%d (all pending)", status.Total))
		if costLine := formatPlanStatusCost(status); costLine != "" {
			r.KV("Cost:", costLine)
		}
		if status.ExecutionPolicy.MaxParallelTasks > 1 {
			r.KV("Parallel:", fmt.Sprintf("%d task(s) per wave", status.ExecutionPolicy.MaxParallelTasks))
		}
		return nil
	}

	r.KV("State:", status.StateFile)
	r.KV("Updated:", fallback(status.UpdatedAt, "-"))
	r.KV("Progress:", fmt.Sprintf("%d/%d tasks done", status.Done, status.Total))
	if costLine := formatPlanStatusCost(status); costLine != "" {
		r.KV("Cost:", costLine)
	}
	if taskLimit := status.ExecutionPolicy.Cost.TaskLimitCents; taskLimit > 0 {
		r.KV("Task Cost Limit:", formatUSDFromMicrosCLI(centsToMicrosCLI(taskLimit)))
	}
	if status.ExecutionPolicy.MaxParallelTasks > 1 {
		r.KV("Parallel:", fmt.Sprintf("%d task(s) per wave", status.ExecutionPolicy.MaxParallelTasks))
	}
	if !costBudgetEnforcedCLI(status.ExecutionPolicy.Cost) {
		r.KV("Cost Mode:", "warn-only")
	}
	if status.Outcome != "" {
		outcome := string(status.Outcome)
		if status.Outcome == domain.RunSuccess {
			outcome = "success"
		}
		if status.Outcome == domain.RunPartial {
			outcome = fmt.Sprintf("partial (%d failed)", status.Failed)
		}
		r.KV("Outcome:", outcome)
	}
	if status.Failed > 0 {
		r.KV("Failed:", fmt.Sprintf("%d", status.Failed))
	}
	r.KV("Status:", planStatusLabel(status))

	if len(status.Tasks) > 0 {
		r.Blank()
		for _, task := range status.Tasks {
			r.CheckItem(taskVariant(task.Status), task.ID, task.Title)
		}
	}
	return nil
}

func taskVariant(status domain.TaskStatus) string {
	switch status {
	case domain.TaskDone:
		return "done"
	case domain.TaskFailed:
		return "fail"
	case domain.TaskExecuting, domain.TaskReviewing:
		return "active"
	default:
		return "pending"
	}
}

func planStatusLabel(status domain.PlanStatus) string {
	if status.StateFile == "" {
		return "not started"
	}
	if status.Running {
		return "running"
	}
	if status.Active == 0 && status.Failed == 0 {
		return "completed"
	}
	if status.Active == 0 && status.Failed > 0 {
		return "failed"
	}
	return "in progress"
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func formatPlanStatusCost(status domain.PlanStatus) string {
	total := formatUSDFromMicrosCLI(status.TotalCostMicros)
	limitCents := status.ExecutionPolicy.Cost.PlanLimitCents
	if limitCents <= 0 {
		if status.TotalCostMicros == 0 {
			return ""
		}
		return total + " (no plan limit)"
	}
	limitMicros := centsToMicrosCLI(limitCents)
	percent := 0.0
	if limitMicros > 0 {
		percent = (float64(status.TotalCostMicros) / float64(limitMicros)) * 100
	}
	return fmt.Sprintf("%s / %s (%.1f%%)", total, formatUSDFromMicrosCLI(limitMicros), percent)
}

func formatUSDFromMicrosCLI(micros int64) string {
	return fmt.Sprintf("$%.4f", float64(micros)/1_000_000)
}

func centsToMicrosCLI(cents int64) int64 {
	if cents <= 0 {
		return 0
	}
	return cents * 10_000
}

func costBudgetEnforcedCLI(policy domain.CostPolicy) bool {
	return policy.Enforce == nil || *policy.Enforce
}

func printPlanRunSummary(out io.Writer, r *Renderer, summary domain.RunSummary) error {
	if summary.TotalTimeS == 0 && summary.TotalCostUSD == 0 && len(summary.ByActor) == 0 &&
		summary.TasksDone == 0 && summary.TasksFailed == 0 && summary.TasksRetried == 0 &&
		summary.Stalls == 0 && summary.Fallbacks == 0 {
		return nil
	}
	r.Header("Run Summary")
	r.KV("Tasks:", fmt.Sprintf("done=%d failed=%d retried=%d", summary.TasksDone, summary.TasksFailed, summary.TasksRetried))
	r.KV("Cost:", fmt.Sprintf("$%.4f", summary.TotalCostUSD))
	r.KV("Time:", fmt.Sprintf("%.1fs", summary.TotalTimeS))
	r.KV("Signals:", fmt.Sprintf("stalls=%d fallbacks=%d", summary.Stalls, summary.Fallbacks))
	if len(summary.ByActor) == 0 {
		return nil
	}

	keys := make([]string, 0, len(summary.ByActor))
	for key := range summary.ByActor {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if _, err := fmt.Fprintln(out, "\nActor                 Calls  Retries  Stalls   Cost       Time"); err != nil {
		return err
	}
	for _, key := range keys {
		stats := summary.ByActor[key]
		if _, err := fmt.Fprintf(out, "%-20s %5d  %7d  %6d  $%0.4f  %0.1fs\n",
			key, stats.Calls, stats.Retries, stats.Stalls, stats.CostUSD, stats.TimeS); err != nil {
			return err
		}
	}
	return nil
}
