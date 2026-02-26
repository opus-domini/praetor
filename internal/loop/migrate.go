package loop

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/opus-domini/praetor/internal/paths"
)

// MigrateLegacyState copies legacy ~/.praetor data to XDG-compliant locations.
// Original files are preserved. If dryRun is true, only prints what would happen.
func MigrateLegacyState(out io.Writer, dryRun bool) error {
	legacyRoot := paths.LegacyRoot()
	if legacyRoot == "" {
		_, _ = fmt.Fprintln(out, "No legacy ~/.praetor directory found. Nothing to migrate.")
		return nil
	}

	configDir, err := paths.DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("resolve XDG config dir: %w", err)
	}
	stateHome, err := paths.DefaultStateHome()
	if err != nil {
		return fmt.Errorf("resolve XDG state home: %w", err)
	}
	cacheHome, err := paths.DefaultCacheHome()
	if err != nil {
		return fmt.Errorf("resolve XDG cache home: %w", err)
	}

	_, _ = fmt.Fprintf(out, "Legacy root:  %s\n", legacyRoot)
	_, _ = fmt.Fprintf(out, "Config dest:  %s\n", configDir)
	_, _ = fmt.Fprintf(out, "State dest:   %s\n", stateHome)
	_, _ = fmt.Fprintf(out, "Cache dest:   %s\n", cacheHome)
	_, _ = fmt.Fprintln(out, "")

	var copied int

	// Migrate config.toml
	legacyConfig := filepath.Join(legacyRoot, "config.toml")
	if _, err := os.Stat(legacyConfig); err == nil {
		dest := filepath.Join(configDir, "config.toml")
		n, err := copyFileIfNeeded(out, legacyConfig, dest, dryRun)
		if err != nil {
			return err
		}
		copied += n
	}

	// Migrate projects directory
	projectsDir := filepath.Join(legacyRoot, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read legacy projects dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectKey := entry.Name()
		legacyProjectDir := filepath.Join(projectsDir, projectKey)

		// State directories -> XDG state
		for _, subdir := range []string{"state", "locks", "costs", "checkpoints", "retries", "feedback"} {
			src := filepath.Join(legacyProjectDir, subdir)
			dest := filepath.Join(stateHome, "projects", projectKey, subdir)
			n, err := copyDirIfNeeded(out, src, dest, dryRun)
			if err != nil {
				return err
			}
			copied += n
		}

		// Logs -> XDG cache
		src := filepath.Join(legacyProjectDir, "logs")
		dest := filepath.Join(cacheHome, "projects", projectKey, "logs")
		n, err := copyDirIfNeeded(out, src, dest, dryRun)
		if err != nil {
			return err
		}
		copied += n
	}

	_, _ = fmt.Fprintln(out, "")
	if dryRun {
		_, _ = fmt.Fprintf(out, "Dry run: %d file(s) would be copied. No changes made.\n", copied)
		_, _ = fmt.Fprintln(out, "Run without --dry-run to perform the migration.")
	} else if copied == 0 {
		_, _ = fmt.Fprintln(out, "Nothing to migrate. XDG locations already up to date.")
	} else {
		_, _ = fmt.Fprintf(out, "Copied %d file(s). Legacy data preserved at %s\n", copied, legacyRoot)
		_, _ = fmt.Fprintln(out, "You can safely remove the legacy directory after verification:")
		_, _ = fmt.Fprintf(out, "  rm -rf %s\n", legacyRoot)
	}

	return nil
}

func copyFileIfNeeded(out io.Writer, src, dest string, dryRun bool) (int, error) {
	if _, err := os.Stat(dest); err == nil {
		return 0, nil // already exists, skip
	}

	_, _ = fmt.Fprintf(out, "  %s -> %s\n", src, dest)
	if dryRun {
		return 1, nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, fmt.Errorf("create directory for %s: %w", dest, err)
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dest, err)
	}
	return 1, nil
}

func copyDirIfNeeded(out io.Writer, src, dest string, dryRun bool) (int, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return 0, nil
	}

	copied := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
		destPath := filepath.Join(dest, relPath)

		n, copyErr := copyFileIfNeeded(out, path, destPath, dryRun)
		if copyErr != nil {
			return copyErr
		}
		copied += n
		return nil
	})
	return copied, err
}
