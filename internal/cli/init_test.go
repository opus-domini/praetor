package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// initInDir runs "praetor init" from the given directory.
// Tests that use t.Chdir must NOT run in parallel.
func initInDir(t *testing.T, dir string, extraArgs ...string) {
	t.Helper()

	t.Chdir(dir)

	args := []string{"init", "--no-color"}
	args = append(args, extraArgs...)

	root := NewRootCmd()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
}

func TestInitCreatesCommandsAndMCP(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	initInDir(t, dir)

	// Verify commands were synced.
	commandsDir := filepath.Join(dir, ".agents", "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		t.Fatalf("read commands dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no commands generated")
	}

	// Verify symlinks exist.
	for _, agent := range []string{"claude", "cursor", "codex"} {
		link := filepath.Join(dir, "."+agent, "commands")
		info, err := os.Lstat(link)
		if err != nil {
			t.Errorf("symlink for %s: %v", agent, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf(".%s/commands is not a symlink", agent)
		}
	}

	// Verify .mcp.json was created.
	mcpPath := filepath.Join(dir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}

	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	entry, ok := cfg.MCPServers["praetor"]
	if !ok {
		t.Fatal("praetor entry not found in .mcp.json")
	}
	if entry.Command != "praetor" {
		t.Errorf("command = %q, want praetor", entry.Command)
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)

	// Run init twice — should not error.
	for range 2 {
		root := NewRootCmd()
		root.SetArgs([]string{"init", "--no-color"})
		if err := root.Execute(); err != nil {
			t.Fatalf("init: %v", err)
		}
	}

	// Verify .mcp.json still valid.
	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse after second run: %v", err)
	}
	if _, ok := cfg.MCPServers["praetor"]; !ok {
		t.Error("praetor entry missing after second run")
	}
}

func TestInitVSCodeDetection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".vscode"), 0o755); err != nil {
		t.Fatal(err)
	}

	initInDir(t, dir)

	// Both .mcp.json and .vscode/mcp.json should exist.
	for _, path := range []string{
		filepath.Join(dir, ".mcp.json"),
		filepath.Join(dir, ".vscode", "mcp.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		var cfg mcpConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		if _, ok := cfg.MCPServers["praetor"]; !ok {
			t.Errorf("praetor entry missing in %s", path)
		}
	}
}

func TestInitMergesExistingMCPConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a pre-existing .mcp.json with another server.
	existing := mcpConfig{
		MCPServers: map[string]mcpServerEntry{
			"other-tool": {Command: "other", Args: []string{"serve"}},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	initInDir(t, dir)

	result, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(result, &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Both entries should be present.
	if _, ok := cfg.MCPServers["praetor"]; !ok {
		t.Error("praetor entry missing")
	}
	if _, ok := cfg.MCPServers["other-tool"]; !ok {
		t.Error("other-tool entry was overwritten")
	}
}
