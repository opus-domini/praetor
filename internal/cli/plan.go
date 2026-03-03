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
	cmd.AddCommand(newPlanDiagnoseCmd())
	return cmd
}

func newPlanCreateCmd() *cobra.Command {
	var fromFile string
	var fromStdin bool
	var dryRun bool
	var noAgent bool
	var slugOverride string
	var plannerAgent string
	var plannerModel string
	var force bool

	cmd := &cobra.Command{
		Use:   "create [brief]",
		Short: "Create a plan from text or markdown input",
		Long:  `Create a plan from a textual brief (arg, file, stdin, or interactive prompt).`,
		Example: `  praetor plan create "Implement JWT auth and tests"
  praetor plan create --from-file docs/brief.md
  cat brief.md | praetor plan create --stdin
  praetor plan create "Refactor billing" --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			out := cmd.OutOrStdout()
			render := NewRenderer(out, false)
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

			brief, err := resolvePlanCreateBrief(args, fromFile, fromStdin, cmd.InOrStdin(), out)
			if err != nil {
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
			if noAgent {
				render.Info("Using template mode (--no-agent).")
				plan = buildNoAgentPlanTemplate(brief, effectivePlanner, effectivePlannerModel)
			} else {
				plannerModelLabel := strings.TrimSpace(effectivePlannerModel)
				if plannerModelLabel == "" {
					plannerModelLabel = "default"
				}
				render.Header("Plan Generation")
				render.KV("Planner:", effectivePlanner)
				render.KV("Model:", plannerModelLabel)
				render.Info("Brief captured. Starting planner generation...")
				plan, err = createPlanWithAgent(cmd.Context(), planCreateAgentRequest{
					Brief:        brief,
					PlannerAgent: domain.Agent(effectivePlanner),
					PlannerModel: effectivePlannerModel,
					ProjectRoot:  projectRoot,
					Config:       cfg,
					Store:        store,
				})
				if err != nil {
					return err
				}
				render.Success("Planner generation completed.")
			}

			author := resolvePlanCreatedBy()
			if err := finalizePlanMetadata(&plan, planFinalizeInput{
				Brief:        brief,
				Source:       ternary(noAgent, "manual", "agent"),
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
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print generated plan JSON without writing file")
	cmd.Flags().BoolVar(&noAgent, "no-agent", false, "Generate a minimal plan template without calling a planner agent")
	cmd.Flags().StringVar(&slugOverride, "slug", "", "Explicit slug override (default: auto-generated from plan name)")
	cmd.Flags().StringVar(&plannerAgent, "planner", "", "Planner agent override (default: config planner or claude)")
	cmd.Flags().StringVar(&plannerModel, "planner-model", "", "Planner model override")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing plan file")
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
			return printPlanStatus(cmd, status)
		},
	}

	return cmd
}

func newPlanListCmd() *cobra.Command {
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
			if len(statuses) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "No plans found in %s\n", store.PlansDir())
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-30s %5s %5s %5s %5s  %s\n", "Plan", "Done", "Fail", "Left", "Total", "Status"); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-30s %5s %5s %5s %5s  %s\n", "----", "----", "----", "----", "-----", "------"); err != nil {
				return err
			}

			for _, status := range statuses {
				label := "in_progress"
				if status.Outcome != "" {
					label = string(status.Outcome)
				}
				if status.Active == 0 && status.Failed == 0 {
					label = "completed"
				} else if status.Active == 0 && status.Failed > 0 {
					label = "failed"
				}
				if status.Running {
					label = "running"
				}
				if status.StateFile == "" {
					label = "not_started"
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-30s %5d %5d %5d %5d  %s\n", status.PlanSlug, status.Done, status.Failed, status.Active, status.Total, label); err != nil {
					return err
				}
			}
			return nil
		},
	}

	return cmd
}

func newPlanResetCmd() *cobra.Command {
	var force bool

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
			if removed == 0 {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Nothing to reset for: %s\n", slug)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Reset complete. Removed %d file(s).\n", removed)
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force reset even if a running lock exists")
	return cmd
}

func newPlanResumeCmd() *cobra.Command {
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

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Resumed from: %s\n", snapshotPath); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State updated: %s\n", store.StateFile(slug)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Progress: %d/%d done\n", snapshot.State.DoneCount(), len(snapshot.State.Tasks))
			return err
		},
	}

	return cmd
}

func newPlanDiagnoseCmd() *cobra.Command {
	var runID string
	var query string
	var format string

	cmd := &cobra.Command{
		Use:     "diagnose <slug>",
		Short:   "Inspect runtime diagnostics for a plan run",
		Example: "  praetor plan diagnose my-plan --query errors\n  praetor plan diagnose my-plan --query costs --format json",
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
			default:
				return fmt.Errorf("unsupported query %q (allowed: errors, stalls, fallbacks, costs, all)", query)
			}
		},
	}

	cmd.Flags().StringVar(&runID, "run-id", "", "Inspect a specific run id (default: latest run for the plan)")
	cmd.Flags().StringVar(&query, "query", "all", "Diagnostic query: errors, stalls, fallbacks, costs, all")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table or json")
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

func resolveStore(projectDir string) (*localstate.Store, error) {
	root, err := localstate.ResolveProjectHome("", projectDir)
	if err != nil {
		return nil, err
	}
	return localstate.NewStore(root), nil
}

type planCreateAgentRequest struct {
	Brief        string
	PlannerAgent domain.Agent
	PlannerModel string
	ProjectRoot  string
	Config       config.Config
	Store        *localstate.Store
}

type planFinalizeInput struct {
	Brief        string
	Source       string
	CreatedBy    string
	PlannerAgent domain.Agent
	PlannerModel string
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

	plan, err := planner.Plan(ctx, pipeline.PlanRequest{
		Objective:      strings.TrimSpace(req.Brief),
		ProjectContext: projectContext,
		Workdir:        req.ProjectRoot,
		Model:          strings.TrimSpace(req.PlannerModel),
		CodexBin:       firstNonEmpty(req.Config.CodexBin, "codex"),
		ClaudeBin:      firstNonEmpty(req.Config.ClaudeBin, "claude"),
	})
	if err != nil {
		var outputErr *pipeline.PlannerOutputError
		if errors.As(err, &outputErr) && strings.TrimSpace(outputErr.RawOutput) != "" {
			logPath, logErr := writePlannerFailureLog(req.Store.LogsDir(), outputErr.RawOutput)
			if logErr == nil {
				return domain.Plan{}, fmt.Errorf("planner generated invalid plan (output logged at %s): %w. Retry with --dry-run or --no-agent", logPath, err)
			}
		}
		return domain.Plan{}, fmt.Errorf("planner failed: %w. Retry with --dry-run or --no-agent", err)
	}

	return plan, nil
}

func writePlannerFailureLog(logRoot, output string) (string, error) {
	ts := time.Now().UTC().Format("20060102-150405")
	dir := filepath.Join(logRoot, "planner-failures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s.log", ts))
	if err := os.WriteFile(path, []byte(strings.TrimSpace(output)+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
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

func printPlanStatus(cmd *cobra.Command, status domain.PlanStatus) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Plan:     %s\n", status.PlanSlug); err != nil {
		return err
	}
	if status.StateFile == "" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "State:    not started"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Tasks:    %d (all pending)\n", status.Total); err != nil {
			return err
		}
		return nil
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State:    %s\n", status.StateFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Updated:  %s\n", fallback(status.UpdatedAt, "-")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Progress: %d/%d tasks done\n", status.Done, status.Total); err != nil {
		return err
	}
	if status.Outcome != "" {
		outcome := string(status.Outcome)
		if status.Outcome == domain.RunSuccess {
			outcome = "success ✓"
		}
		if status.Outcome == domain.RunPartial {
			outcome = fmt.Sprintf("partial (%d failed)", status.Failed)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Outcome:  %s\n", outcome); err != nil {
			return err
		}
	}
	if status.Failed > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Failed:   %d\n", status.Failed); err != nil {
			return err
		}
	}
	stateLabel := "in progress"
	if status.Active == 0 && status.Failed == 0 {
		stateLabel = "completed"
	} else if status.Active == 0 && status.Failed > 0 {
		stateLabel = "failed"
	}
	if status.Running {
		stateLabel = "running"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Status:   %s\n", stateLabel); err != nil {
		return err
	}

	if len(status.Tasks) > 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), ""); err != nil {
			return err
		}
		for _, task := range status.Tasks {
			mark := taskStatusMark(task.Status)
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", mark, task.ID, task.Title); err != nil {
				return err
			}
		}
	}
	return nil
}

func taskStatusMark(status domain.TaskStatus) string {
	switch status {
	case domain.TaskDone:
		return "x"
	case domain.TaskFailed:
		return "!"
	case domain.TaskExecuting, domain.TaskReviewing:
		return ">"
	default:
		return " "
	}
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
