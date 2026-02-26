package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigDirPraetorHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := DefaultConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tmp, "config") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "config"), dir)
	}
}

func TestDefaultConfigDirXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := DefaultConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tmp, "praetor") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "praetor"), dir)
	}
}

func TestDefaultConfigDirFallback(t *testing.T) {
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	dir, err := DefaultConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, filepath.Join("praetor")) {
		t.Fatalf("expected path ending in praetor, got %s", dir)
	}
}

func TestDefaultConfigFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)

	file, err := DefaultConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmp, "config", "config.toml")
	if file != expected {
		t.Fatalf("expected %s, got %s", expected, file)
	}
}

func TestDefaultStateHomePraetorHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := DefaultStateHome()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tmp, "state") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "state"), dir)
	}
}

func TestDefaultStateHomeXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_STATE_HOME", tmp)

	dir, err := DefaultStateHome()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tmp, "praetor") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "praetor"), dir)
	}
}

func TestDefaultStateHomeFallback(t *testing.T) {
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	dir, err := DefaultStateHome()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "praetor")
	if dir != expected {
		t.Fatalf("expected %s, got %s", expected, dir)
	}
}

func TestDefaultCacheHomePraetorHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", "")

	dir, err := DefaultCacheHome()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tmp, "cache") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "cache"), dir)
	}
}

func TestDefaultCacheHomeXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_CACHE_HOME", tmp)

	dir, err := DefaultCacheHome()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(tmp, "praetor") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "praetor"), dir)
	}
}

func TestDefaultProjectStateRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)

	root, err := DefaultProjectStateRoot("/home/user/myproject")
	if err != nil {
		t.Fatal(err)
	}
	key := ProjectRuntimeKey("/home/user/myproject")
	expected := filepath.Join(tmp, "state", "projects", key)
	if root != expected {
		t.Fatalf("expected %s, got %s", expected, root)
	}
}

func TestDefaultProjectCacheRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)

	root, err := DefaultProjectCacheRoot("/home/user/myproject")
	if err != nil {
		t.Fatal(err)
	}
	key := ProjectRuntimeKey("/home/user/myproject")
	expected := filepath.Join(tmp, "cache", "projects", key)
	if root != expected {
		t.Fatalf("expected %s, got %s", expected, root)
	}
}

func TestProjectRuntimeKeyDeterministic(t *testing.T) {
	t.Parallel()
	key1 := ProjectRuntimeKey("/home/user/project")
	key2 := ProjectRuntimeKey("/home/user/project")
	if key1 != key2 {
		t.Fatalf("expected deterministic keys, got %s and %s", key1, key2)
	}
}

func TestProjectRuntimeKeyDiffersForDifferentPaths(t *testing.T) {
	t.Parallel()
	keyA := ProjectRuntimeKey("/home/user/project-a")
	keyB := ProjectRuntimeKey("/home/user/project-b")
	if keyA == keyB {
		t.Fatal("expected different keys for different paths")
	}
}

func TestProjectRuntimeKeyContainsBasename(t *testing.T) {
	t.Parallel()
	key := ProjectRuntimeKey("/home/user/my-project")
	if !strings.HasPrefix(key, "my-project-") {
		t.Fatalf("expected key to start with basename, got %s", key)
	}
}

func TestPraetorHomeTakesPrecedenceOverXDG(t *testing.T) {
	praetorHome := t.TempDir()
	xdgConfig := t.TempDir()
	xdgState := t.TempDir()
	xdgCache := t.TempDir()

	t.Setenv("PRAETOR_HOME", praetorHome)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_STATE_HOME", xdgState)
	t.Setenv("XDG_CACHE_HOME", xdgCache)

	configDir, _ := DefaultConfigDir()
	stateHome, _ := DefaultStateHome()
	cacheHome, _ := DefaultCacheHome()

	if configDir != filepath.Join(praetorHome, "config") {
		t.Fatalf("PRAETOR_HOME should take precedence for config, got %s", configDir)
	}
	if stateHome != filepath.Join(praetorHome, "state") {
		t.Fatalf("PRAETOR_HOME should take precedence for state, got %s", stateHome)
	}
	if cacheHome != filepath.Join(praetorHome, "cache") {
		t.Fatalf("PRAETOR_HOME should take precedence for cache, got %s", cacheHome)
	}
}
