package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runInit runs "praetor init" from the given directory and returns stdout.
func runInit(t *testing.T, dir string, extraArgs ...string) string {
	t.Helper()
	t.Chdir(dir)

	args := []string{"init", "--no-color"}
	args = append(args, extraArgs...)

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	return out.String()
}

func TestInitCreatesCommandsAndMCP(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	output := runInit(t, dir)

	// Verify structured output.
	if !strings.Contains(output, "Installing into") {
		t.Error("missing banner")
	}
	if !strings.Contains(output, "Scanning project") {
		t.Error("missing scan phase")
	}
	if !strings.Contains(output, "Agent Commands") {
		t.Error("missing agent commands step")
	}
	if !strings.Contains(output, "MCP Server") {
		t.Error("missing MCP step")
	}
	if !strings.Contains(output, "Praetor is ready!") {
		t.Error("missing completion message")
	}
	if !strings.Contains(output, "Next steps") {
		t.Error("missing next steps")
	}

	// Verify commands were synced.
	commandsDir := filepath.Join(dir, ".agents", "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		t.Fatalf("read commands dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("no commands generated")
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "praetor-") {
			t.Errorf("expected prefixed command file, got %q", entry.Name())
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

func TestInitDetectsExistingAgentDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only create .claude/ — init should detect only claude.
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	output := runInit(t, dir)

	if !strings.Contains(output, "Detected agents: claude") {
		t.Errorf("expected detection of claude, got:\n%s", output)
	}

	// Symlink should exist for claude.
	link := filepath.Join(dir, ".claude", "commands")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("symlink for claude: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error(".claude/commands is not a symlink")
	}
}

func TestInitUsesDefaultsWhenNoAgentDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	output := runInit(t, dir)

	if !strings.Contains(output, "No agent directories found") {
		t.Errorf("expected defaults message, got:\n%s", output)
	}

	// All default agents should get symlinks.
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

	// Second run should show "Already registered".
	var out bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{"init", "--no-color"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init third run: %v", err)
	}
	if !strings.Contains(out.String(), "Already registered") {
		t.Error("expected 'Already registered' on repeat run")
	}

	// Verify .mcp.json still valid.
	data, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := cfg.MCPServers["praetor"]; !ok {
		t.Error("praetor entry missing after repeat run")
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

	output := runInit(t, dir)

	if !strings.Contains(output, ".vscode/mcp.json") {
		t.Errorf("expected VS Code target in output, got:\n%s", output)
	}

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

	runInit(t, dir)

	result, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(result, &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if _, ok := cfg.MCPServers["praetor"]; !ok {
		t.Error("praetor entry missing")
	}
	if _, ok := cfg.MCPServers["other-tool"]; !ok {
		t.Error("other-tool entry was overwritten")
	}
}

func TestInitNoConfigInit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	output := runInit(t, dir)

	// init should NOT touch global config.
	if strings.Contains(output, "config.toml") {
		t.Error("init should not create or reference global config")
	}
	if strings.Contains(output, "Config created") {
		t.Error("init should not run config init")
	}
}
