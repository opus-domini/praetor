package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and manage configuration",
		Long:  "Inspect the resolved config cascade (defaults < global < project) and set values without editing TOML by hand.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigSetCmd())
	cmd.AddCommand(newConfigPathCmd())
	cmd.AddCommand(newConfigEditCmd())
	cmd.AddCommand(newConfigInitCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	var workdir string
	var noColor bool

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show effective configuration",
		Long:  "Display all config keys with their effective values and source annotations (default, config, project).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true

			projectRoot := strings.TrimSpace(workdir)
			resolved, cfgPath, err := config.LoadResolved(projectRoot)
			if err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), noColor)
			groups := groupResolved(resolved)
			for _, g := range groups {
				r.Header(string(g.category))
				for _, rv := range g.values {
					display := rv.Value
					if display == "" {
						display = "-"
					}
					r.ConfigKV(rv.Key, display, string(rv.Source))
				}
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			if cfgPath != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", cfgPath)
			}
			if projectRoot != "" {
				abs, absErr := filepath.Abs(projectRoot)
				if absErr == nil {
					projectRoot = abs
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Project: %s\n", projectRoot)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&workdir, "workdir", "", "Project root for project-level overrides")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	return cmd
}

type resolvedGroup struct {
	category config.Category
	values   []config.ResolvedValue
}

func groupResolved(resolved []config.ResolvedValue) []resolvedGroup {
	groups := make(map[config.Category][]config.ResolvedValue)
	for _, rv := range resolved {
		groups[rv.Meta.Category] = append(groups[rv.Meta.Category], rv)
	}
	var result []resolvedGroup
	for _, cat := range config.CategoryOrder {
		if vals, ok := groups[cat]; ok {
			result = append(result, resolvedGroup{category: cat, values: vals})
		}
	}
	return result
}

func newConfigSetCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config key",
		Long:  "Set a config key and persist to the config file. Use --project to write into a project section.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			key := strings.TrimSpace(args[0])
			value := strings.TrimSpace(args[1])

			cfgPath := config.Path()
			if cfgPath == "" {
				return errors.New("cannot determine config file path")
			}

			section := strings.TrimSpace(project)
			if err := config.SetValue(cfgPath, section, key, value); err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), false)
			where := "global"
			if section != "" {
				where = section
			}
			r.Success(fmt.Sprintf("%s = %s [%s]", key, value, where))
			return nil
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "Write into a project section instead of global")
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print resolved config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			path := config.Path()
			if path == "" {
				return errors.New("cannot determine config file path")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open config file in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			path := config.Path()
			if path == "" {
				return errors.New("cannot determine config file path")
			}
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				r := NewRenderer(cmd.OutOrStdout(), false)
				r.Warn("Config file not found. Run 'praetor config init' to create one.")
				return nil
			}
			return openEditor(path)
		},
	}
}

func newConfigInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a commented template config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			path := config.Path()
			if path == "" {
				return errors.New("cannot determine config file path")
			}

			if !force {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("config file already exists: %s (use --force to overwrite)", path)
				}
			}

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create config directory: %w", err)
			}

			if err := os.WriteFile(path, []byte(config.Template()), 0o644); err != nil {
				return err
			}

			r := NewRenderer(cmd.OutOrStdout(), false)
			r.Success(fmt.Sprintf("Config created: %s", path))
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config file")
	return cmd
}
