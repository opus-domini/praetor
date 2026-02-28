package state

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// DefaultHome returns the praetor home directory.
// Resolution: $PRAETOR_HOME > $XDG_CONFIG_HOME/praetor > ~/.config/praetor
func DefaultHome() (string, error) {
	if home := strings.TrimSpace(os.Getenv("PRAETOR_HOME")); home != "" {
		return home, nil
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
	home, err := DefaultHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "config.toml"), nil
}

// DefaultProjectRoot returns the per-project directory under the praetor home.
func DefaultProjectRoot(projectRoot string) (string, error) {
	home, err := DefaultHome()
	if err != nil {
		return "", err
	}
	key := ProjectRuntimeKey(projectRoot)
	return filepath.Join(home, "projects", key), nil
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
