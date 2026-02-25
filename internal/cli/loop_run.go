package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/loop"
	"github.com/spf13/cobra"
)

func newLoopRunCmd() *cobra.Command {
	var planFile string
	var stateRoot string
	var workdir string
	var defaultExecutor string
	var defaultReviewer string
	var maxRetries int
	var maxIterations int
	var skipReview bool
	var force bool
	var codexBin string
	var claudeBin string
	var tmuxSession string
	var noColor bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a task plan with executor/reviewer orchestration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			planFile = strings.TrimSpace(planFile)
			if planFile == "" {
				return errors.New("--plan is required")
			}

			absPlan, err := filepath.Abs(planFile)
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

			runner := loop.NewRunner(nil)
			runnerOptions := loop.RunnerOptions{
				StateRoot:       resolvedStateRoot,
				Workdir:         absWorkdir,
				DefaultExecutor: loop.Agent(defaultExecutor),
				DefaultReviewer: loop.Agent(defaultReviewer),
				MaxRetries:      maxRetries,
				MaxIterations:   maxIterations,
				SkipReview:      skipReview,
				Force:           force,
				CodexBin:        codexBin,
				ClaudeBin:       claudeBin,
				TMUXSession:     tmuxSession,
				NoColor:         noColor,
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

	cmd.Flags().StringVar(&planFile, "plan", "", "Plan JSON file path (required)")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "Runtime state root directory (default: ~/.praetor/projects/<project-hash>)")
	cmd.Flags().StringVar(&workdir, "workdir", ".", "Working directory for agents")
	cmd.Flags().StringVar(&defaultExecutor, "default-executor", string(loop.AgentCodex), "Default executor agent: codex or claude")
	cmd.Flags().StringVar(&defaultReviewer, "default-reviewer", string(loop.AgentClaude), "Default reviewer agent: codex, claude or none")
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum retries per task")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 0, "Maximum loop iterations (0 means unlimited)")
	cmd.Flags().BoolVar(&skipReview, "skip-review", false, "Skip reviewer gate and auto-approve successful executor outputs")
	cmd.Flags().BoolVar(&force, "force", false, "Override existing plan lock")
	cmd.Flags().StringVar(&codexBin, "codex-bin", "codex", "Codex command path or name")
	cmd.Flags().StringVar(&claudeBin, "claude-bin", "claude", "Claude command path or name")
	cmd.Flags().StringVar(&tmuxSession, "tmux-session", "", "tmux session name (default: praetor-<project-hash>)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Optional timeout, for example: 30m")
	return cmd
}
