package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/loop"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var stateRoot string
	var workdir string
	var executor string
	var reviewer string
	var maxRetries int
	var maxIterations int
	var noReview bool
	var force bool
	var codexBin string
	var claudeBin string
	var tmuxSession string
	var noColor bool
	var isolation string
	var postTaskHook string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "run <plan-file>",
		Short: "Execute a task plan",
		Long: `Execute a task plan with executor/reviewer orchestration.

Each task runs an executor agent, then an independent reviewer agent
that gates promotion. Failed tasks are retried with feedback. Worktree
isolation protects the main branch from partial changes.`,
		Example: `  praetor run docs/plans/my-plan.json
  praetor run docs/plans/my-plan.json --executor claude --reviewer claude
  praetor run docs/plans/my-plan.json --hook ./scripts/lint.sh --timeout 1h`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			absPlan, err := filepath.Abs(strings.TrimSpace(args[0]))
			if err != nil {
				return fmt.Errorf("resolve plan path: %w", err)
			}

			absWorkdir := strings.TrimSpace(workdir)
			if absWorkdir != "" {
				absWorkdir, err = filepath.Abs(absWorkdir)
				if err != nil {
					return fmt.Errorf("resolve workdir path: %w", err)
				}
			}

			resolvedStateRoot, err := resolveStateRoot(stateRoot, absWorkdir)
			if err != nil {
				return err
			}

			// Load user config and apply defaults for unset flags.
			cfg, cfgErr := config.Load(absWorkdir)
			if cfgErr != nil {
				return cfgErr
			}
			f := cmd.Flags()
			if !f.Changed("executor") && cfg.Executor != "" {
				executor = cfg.Executor
			}
			if !f.Changed("reviewer") && cfg.Reviewer != "" {
				reviewer = cfg.Reviewer
			}
			if !f.Changed("max-retries") && cfg.MaxRetries != nil {
				maxRetries = *cfg.MaxRetries
			}
			if !f.Changed("max-iterations") && cfg.MaxIterations != nil {
				maxIterations = *cfg.MaxIterations
			}
			if !f.Changed("no-review") && cfg.NoReview != nil {
				noReview = *cfg.NoReview
			}
			if !f.Changed("no-color") && cfg.NoColor != nil {
				noColor = *cfg.NoColor
			}
			if !f.Changed("isolation") && cfg.Isolation != "" {
				isolation = cfg.Isolation
			}
			if !f.Changed("codex-bin") && cfg.CodexBin != "" {
				codexBin = cfg.CodexBin
			}
			if !f.Changed("claude-bin") && cfg.ClaudeBin != "" {
				claudeBin = cfg.ClaudeBin
			}
			if !f.Changed("hook") && cfg.Hook != "" {
				postTaskHook = cfg.Hook
			}
			if !f.Changed("timeout") && cfg.Timeout != "" {
				if d, parseErr := time.ParseDuration(cfg.Timeout); parseErr == nil {
					timeout = d
				}
			}

			runner := loop.NewRunner(nil)
			runnerOptions := loop.RunnerOptions{
				StateRoot:       resolvedStateRoot,
				Workdir:         absWorkdir,
				DefaultExecutor: loop.Agent(executor),
				DefaultReviewer: loop.Agent(reviewer),
				MaxRetries:      maxRetries,
				MaxIterations:   maxIterations,
				SkipReview:      noReview,
				Force:           force,
				CodexBin:        codexBin,
				ClaudeBin:       claudeBin,
				TMUXSession:     tmuxSession,
				NoColor:         noColor,
				Isolation:       loop.IsolationMode(strings.ToLower(strings.TrimSpace(isolation))),
				PostTaskHook:    postTaskHook,
			}

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			stats, err := runner.Run(ctx, cmd.OutOrStdout(), absPlan, runnerOptions)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "State saved at: %s\n", stats.StateFile)
			return err
		},
	}

	cmd.Flags().StringVar(&executor, "executor", string(loop.AgentCodex), "Default executor agent: codex or claude")
	cmd.Flags().StringVar(&reviewer, "reviewer", string(loop.AgentClaude), "Default reviewer agent: codex, claude, or none")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum retries per task (must be > 0)")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 0, "Maximum loop iterations (0 = unlimited)")
	cmd.Flags().BoolVar(&noReview, "no-review", false, "Skip the reviewer gate and auto-approve executor outputs")
	cmd.Flags().BoolVar(&force, "force", false, "Override an existing plan lock")
	cmd.Flags().StringVar(&codexBin, "codex-bin", "codex", "Codex binary path or name")
	cmd.Flags().StringVar(&claudeBin, "claude-bin", "claude", "Claude binary path or name")
	cmd.Flags().StringVar(&tmuxSession, "tmux-session", "", "tmux session name (default: praetor-<project-hash>)")
	cmd.Flags().StringVar(&workdir, "workdir", ".", "Working directory for agents")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "State root directory (default: ~/.praetor/projects/<hash>)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().StringVar(&isolation, "isolation", string(loop.IsolationWorktree), "Isolation mode: worktree or off")
	cmd.Flags().StringVar(&postTaskHook, "hook", "", "Script to run after executor, before reviewer")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Run timeout (e.g. 30m, 2h)")
	return cmd
}
