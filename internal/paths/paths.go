package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfigDir returns the directory for user configuration.
// Resolution: $PRAETOR_HOME/config > $XDG_CONFIG_HOME/praetor > os.UserConfigDir()/praetor
func DefaultConfigDir() (string, error) {
	if home := strings.TrimSpace(os.Getenv("PRAETOR_HOME")); home != "" {
		return filepath.Join(home, "config"), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "praetor"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "praetor"), nil
}

// DefaultConfigFile returns the full path to config.toml.
func DefaultConfigFile() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// DefaultStateHome returns the root for persistent machine-generated state.
// Resolution: $PRAETOR_HOME/state > $XDG_STATE_HOME/praetor > ~/.local/state/praetor
func DefaultStateHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("PRAETOR_HOME")); home != "" {
		return filepath.Join(home, "state"), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		return filepath.Join(xdg, "praetor"), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".local", "state", "praetor"), nil
}

// DefaultCacheHome returns the root for purgeable runtime artifacts.
// Resolution: $PRAETOR_HOME/cache > $XDG_CACHE_HOME/praetor > os.UserCacheDir()/praetor
func DefaultCacheHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("PRAETOR_HOME")); home != "" {
		return filepath.Join(home, "cache"), nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return filepath.Join(xdg, "praetor"), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "praetor"), nil
}

// DefaultProjectStateRoot returns the per-project state directory.
func DefaultProjectStateRoot(projectRoot string) (string, error) {
	stateHome, err := DefaultStateHome()
	if err != nil {
		return "", err
	}
	key := ProjectRuntimeKey(projectRoot)
	return filepath.Join(stateHome, "projects", key), nil
}

// DefaultProjectCacheRoot returns the per-project cache directory.
func DefaultProjectCacheRoot(projectRoot string) (string, error) {
	cacheHome, err := DefaultCacheHome()
	if err != nil {
		return "", err
	}
	key := ProjectRuntimeKey(projectRoot)
	return filepath.Join(cacheHome, "projects", key), nil
}

// LegacyRoot returns ~/.praetor if it exists, empty string otherwise.
func LegacyRoot() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(homeDir, ".praetor")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// LegacyConfigFile returns ~/.praetor/config.toml if it exists.
func LegacyConfigFile() string {
	root := LegacyRoot()
	if root == "" {
		return ""
	}
	path := filepath.Join(root, "config.toml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// LegacyProjectStateRoot returns ~/.praetor/projects/<key> if it exists.
func LegacyProjectStateRoot(projectRoot string) string {
	root := LegacyRoot()
	if root == "" {
		return ""
	}
	key := ProjectRuntimeKey(projectRoot)
	dir := filepath.Join(root, "projects", key)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

// ProjectRuntimeKey returns the collision-resistant key for a project root path.
// Format: <basename>-<sha256[:12]>
func ProjectRuntimeKey(projectRoot string) string {
	baseName := strings.TrimSpace(filepath.Base(projectRoot))
	baseName = strings.Trim(baseName, ".")
	if baseName == "" {
		baseName = "project"
	}

	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "\t", "-", "\n", "-", "\r", "-")
	baseName = replacer.Replace(baseName)

	hash := sha256.Sum256([]byte(projectRoot))
	hashPart := hex.EncodeToString(hash[:])[:12]
	return baseName + "-" + hashPart
}
