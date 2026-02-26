package state

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultHomePraetorHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", "")

	home, err := DefaultHome()
	if err != nil {
		t.Fatal(err)
	}
	if home != tmp {
		t.Fatalf("expected %s, got %s", tmp, home)
	}
}

func TestDefaultHomeXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", tmp)

	home, err := DefaultHome()
	if err != nil {
		t.Fatal(err)
	}
	if home != filepath.Join(tmp, "praetor") {
		t.Fatalf("expected %s, got %s", filepath.Join(tmp, "praetor"), home)
	}
}

func TestDefaultHomeFallback(t *testing.T) {
	t.Setenv("PRAETOR_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	home, err := DefaultHome()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(home, filepath.Join("praetor")) {
		t.Fatalf("expected path ending in praetor, got %s", home)
	}
}

func TestDefaultConfigFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)

	file, err := DefaultConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmp, "config.toml")
	if file != expected {
		t.Fatalf("expected %s, got %s", expected, file)
	}
}

func TestDefaultProjectRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PRAETOR_HOME", tmp)

	root, err := DefaultProjectRoot("/home/user/myproject")
	if err != nil {
		t.Fatal(err)
	}
	key := ProjectRuntimeKey("/home/user/myproject")
	expected := filepath.Join(tmp, "projects", key)
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

	t.Setenv("PRAETOR_HOME", praetorHome)
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)

	home, _ := DefaultHome()

	if home != praetorHome {
		t.Fatalf("PRAETOR_HOME should take precedence, got %s", home)
	}
}
