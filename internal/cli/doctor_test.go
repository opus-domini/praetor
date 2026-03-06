package cli

import (
	"bytes"
	"strings"
	"testing"

	agent "github.com/opus-domini/praetor/internal/agent"
	"github.com/opus-domini/praetor/internal/config"
)

func TestDoctorCommandCreation(t *testing.T) {
	t.Parallel()

	cmd := newDoctorCmd()
	if cmd.Use != "doctor" {
		t.Errorf("expected Use='doctor', got %q", cmd.Use)
	}

	// Verify expected flags exist.
	flags := []string{"workdir", "no-color", "timeout"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag %q to exist", name)
		}
	}
}

func TestDoctorCommandRunsWithoutError(t *testing.T) {
	// Cannot use t.Parallel with t.Setenv.
	t.Setenv("PRAETOR_CONFIG", "/nonexistent/config.toml")

	cmd := newDoctorCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--no-color", "--timeout", "2s"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("doctor command failed: %v", err)
	}

	output := buf.String()

	// Should contain the header.
	if !strings.Contains(output, "Agent Health Check") {
		t.Error("output should contain 'Agent Health Check' header")
	}

	// Should contain all agent display names.
	for _, entry := range agent.AllCatalogEntries() {
		if !strings.Contains(output, entry.DisplayName) {
			t.Errorf("output should contain %q", entry.DisplayName)
		}
	}

	// Should contain the summary line.
	if !strings.Contains(output, "agents available") {
		t.Error("output should contain 'agents available' summary")
	}
}

func TestDoctorOutputContainsTransportTags(t *testing.T) {
	// Cannot use t.Parallel with t.Setenv.
	t.Setenv("PRAETOR_CONFIG", "/nonexistent/config.toml")

	cmd := newDoctorCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--no-color", "--timeout", "2s"})

	_ = cmd.Execute()
	output := buf.String()

	if !strings.Contains(output, "[CLI]") {
		t.Error("output should contain [CLI] transport tags")
	}
	if !strings.Contains(output, "[REST]") {
		t.Error("output should contain [REST] transport tags")
	}
}

func TestWriteProbeProgressAndClear(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	r := NewRenderer(buf, true) // no color

	entry := agent.CatalogEntry{
		DisplayName: "Claude Code",
	}

	writeProbeProgress(buf, r, entry)
	output := buf.String()
	if !strings.Contains(output, "Checking Claude Code...") {
		t.Errorf("expected progress message, got %q", output)
	}
	if !strings.HasSuffix(output, "\r") {
		t.Error("progress line should end with carriage return")
	}

	buf.Reset()
	clearProbeProgress(buf, r, entry)
	cleared := buf.String()
	if !strings.HasPrefix(cleared, "\r") {
		t.Error("clear should start with carriage return")
	}
	if !strings.HasSuffix(cleared, "\r") {
		t.Error("clear should end with carriage return")
	}
}

func TestBuildBinaryOverrides(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		CodexBin:    "custom-codex",
		ClaudeBin:   "custom-claude",
		CopilotBin:  "custom-copilot",
		GeminiBin:   "custom-gemini",
		KimiBin:     "custom-kimi",
		OpenCodeBin: "custom-opencode",
	}

	overrides := buildBinaryOverrides(cfg)

	expected := map[agent.ID]string{
		agent.Codex:    "custom-codex",
		agent.Claude:   "custom-claude",
		agent.Copilot:  "custom-copilot",
		agent.Gemini:   "custom-gemini",
		agent.Kimi:     "custom-kimi",
		agent.OpenCode: "custom-opencode",
	}

	for id, want := range expected {
		got, ok := overrides[id]
		if !ok {
			t.Errorf("expected override for %q", id)
			continue
		}
		if got != want {
			t.Errorf("override[%q] = %q, want %q", id, got, want)
		}
	}
}

func TestBuildBinaryOverridesEmpty(t *testing.T) {
	t.Parallel()

	overrides := buildBinaryOverrides(config.Config{})
	if len(overrides) != 0 {
		t.Errorf("expected empty overrides, got %d entries", len(overrides))
	}
}

func TestBuildRESTEndpoints(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		OpenRouterURL: "https://custom.openrouter.ai/v1",
		OllamaURL:     "http://192.168.1.100:11434",
	}

	endpoints := buildRESTEndpoints(cfg)
	if endpoints[agent.OpenRouter] != "https://custom.openrouter.ai/v1" {
		t.Errorf("expected OpenRouter URL, got %q", endpoints[agent.OpenRouter])
	}
	if endpoints[agent.Ollama] != "http://192.168.1.100:11434" {
		t.Errorf("expected Ollama URL, got %q", endpoints[agent.Ollama])
	}
}

func TestBuildRESTEndpointsEmpty(t *testing.T) {
	t.Parallel()

	endpoints := buildRESTEndpoints(config.Config{})
	if len(endpoints) != 0 {
		t.Errorf("expected empty endpoints, got %d entries", len(endpoints))
	}
}

func TestWriteProbeResultFormatsCorrectly(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	r := NewRenderer(buf, true) // no color for testing

	writeProbeResult(buf, r, agent.ProbeResult{
		ID:          agent.Claude,
		DisplayName: "Claude Code",
		Transport:   agent.TransportCLI,
		Status:      agent.StatusPass,
		Version:     "1.0.30",
		Path:        "/usr/local/bin/claude",
		Detail:      "v1.0.30",
	})

	output := buf.String()
	if !strings.Contains(output, "Claude Code") {
		t.Error("output should contain display name")
	}
	if !strings.Contains(output, "[CLI]") {
		t.Error("output should contain transport tag")
	}
	if !strings.Contains(output, "v1.0.30") {
		t.Error("output should contain version detail")
	}
	if !strings.Contains(output, "/usr/local/bin/claude") {
		t.Error("output should contain binary path")
	}
}

func TestWriteProbeResultNotFound(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	r := NewRenderer(buf, true) // no color

	writeProbeResult(buf, r, agent.ProbeResult{
		ID:          agent.Kimi,
		DisplayName: "Kimi CLI",
		Transport:   agent.TransportCLI,
		Status:      agent.StatusFail,
		Detail:      "kimi not found in PATH",
	})

	output := buf.String()
	if !strings.Contains(output, "Kimi CLI") {
		t.Error("output should contain display name")
	}
	if !strings.Contains(output, "not found in PATH") {
		t.Error("output should contain error detail")
	}
	// Should always have a second line with install hint.
	if !strings.Contains(output, "kimi.ai") {
		t.Error("output should contain install hint on second line")
	}
}

func TestWriteProbeResultREST(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	r := NewRenderer(buf, true) // no color

	writeProbeResult(buf, r, agent.ProbeResult{
		ID:          agent.Ollama,
		DisplayName: "Ollama",
		Transport:   agent.TransportREST,
		Status:      agent.StatusPass,
		Path:        "http://127.0.0.1:11434",
		Detail:      "reachable (HTTP 200)",
	})

	output := buf.String()
	if !strings.Contains(output, "Ollama") {
		t.Error("output should contain display name")
	}
	if !strings.Contains(output, "[REST]") {
		t.Error("output should contain REST transport tag")
	}
	if !strings.Contains(output, "reachable") {
		t.Error("output should contain reachability info")
	}
}
