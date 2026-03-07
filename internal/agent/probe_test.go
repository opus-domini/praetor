package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestProbeCLIBinaryFound(t *testing.T) {
	t.Parallel()

	// Use a deterministic fake binary instead of relying on system shell
	// semantics, which vary across CI environments.
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "probe-pass")
	script := "#!/bin/sh\necho 'probe-pass version 1.0.0'\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	prober := NewProber(WithTimeout(5 * time.Second))
	result := prober.probeCLI(context.Background(), CatalogEntry{
		ID:          "test-shell",
		DisplayName: "Test Shell",
		Transport:   TransportCLI,
		Binary:      fakeBin,
		VersionArgs: []string{"--version"},
	}, fakeBin)

	if result.Status != StatusPass {
		t.Errorf("expected StatusPass, got %q: %s", result.Status, result.Detail)
	}
	if result.Path == "" {
		t.Error("expected non-empty path")
	}
}

func TestProbeCLIBinaryNotFound(t *testing.T) {
	t.Parallel()

	prober := NewProber(WithTimeout(2 * time.Second))
	result := prober.probeCLI(context.Background(), CatalogEntry{
		ID:          "nonexistent",
		DisplayName: "Nonexistent Tool",
		Transport:   TransportCLI,
		Binary:      "definitely-not-a-real-binary-xyz",
		VersionArgs: []string{"--version"},
	}, "definitely-not-a-real-binary-xyz")

	if result.Status != StatusFail {
		t.Errorf("expected StatusFail, got %q: %s", result.Status, result.Detail)
	}
	if result.Path != "" {
		t.Error("expected empty path for not-found binary")
	}
}

func TestProbeCLIVersionParseFromFakeBinary(t *testing.T) {
	t.Parallel()

	// Create a fake binary that outputs a version.
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "fake-agent")
	script := "#!/bin/sh\necho 'fake-agent version 1.2.3'\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	prober := NewProber(WithTimeout(5 * time.Second))
	result := prober.probeCLI(context.Background(), CatalogEntry{
		ID:          "fake",
		DisplayName: "Fake Agent",
		Transport:   TransportCLI,
		Binary:      fakeBin,
		VersionArgs: []string{"--version"},
	}, fakeBin)

	if result.Status != StatusPass {
		t.Fatalf("expected StatusPass, got %q: %s", result.Status, result.Detail)
	}
	if result.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got %q", result.Version)
	}
}

func TestProbeCLIVersionCommandFails(t *testing.T) {
	t.Parallel()

	// Create a fake binary that exits non-zero.
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "failing-agent")
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	prober := NewProber(WithTimeout(5 * time.Second))
	result := prober.probeCLI(context.Background(), CatalogEntry{
		ID:          "failing",
		DisplayName: "Failing Agent",
		Transport:   TransportCLI,
		Binary:      fakeBin,
		VersionArgs: []string{"--version"},
	}, fakeBin)

	// Binary exists but version fails — still considered usable but degraded.
	if result.Status != StatusWarn {
		t.Errorf("expected StatusWarn (binary exists despite version failure), got %q: %s", result.Status, result.Detail)
	}
	if result.Version != "" {
		t.Errorf("expected empty version on failure, got %q", result.Version)
	}
}

func TestProbeCLINoVersionArgs(t *testing.T) {
	t.Parallel()

	binary := "sh"
	if runtime.GOOS == "windows" {
		binary = "cmd"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("test binary %q not in PATH: %v", binary, err)
	}

	prober := NewProber(WithTimeout(5 * time.Second))
	result := prober.probeCLI(context.Background(), CatalogEntry{
		ID:          "noversionargs",
		DisplayName: "No Version Args",
		Transport:   TransportCLI,
		Binary:      binary,
		VersionArgs: nil, // no version detection
	}, binary)

	if result.Status != StatusPass {
		t.Errorf("expected StatusPass, got %q: %s", result.Status, result.Detail)
	}
}

func TestProbeRESTHealthy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	prober := NewProber(
		WithHTTPClient(srv.Client()),
		WithTimeout(5*time.Second),
	)
	result := prober.probeREST(context.Background(), CatalogEntry{
		ID:             "test-rest",
		DisplayName:    "Test REST",
		Transport:      TransportREST,
		HealthEndpoint: "/health",
	}, srv.URL)

	if result.Status != StatusPass {
		t.Errorf("expected StatusPass, got %q: %s", result.Status, result.Detail)
	}
	if result.Path != srv.URL {
		t.Errorf("expected path %q, got %q", srv.URL, result.Path)
	}
}

func TestProbeRESTUnreachable(t *testing.T) {
	t.Parallel()

	prober := NewProber(
		WithHTTPClient(&http.Client{Timeout: 1 * time.Second}),
		WithTimeout(2*time.Second),
	)
	result := prober.probeREST(context.Background(), CatalogEntry{
		ID:             "unreachable",
		DisplayName:    "Unreachable",
		Transport:      TransportREST,
		HealthEndpoint: "/health",
	}, "http://127.0.0.1:1") // port 1 is virtually always unreachable

	if result.Status != StatusFail {
		t.Errorf("expected StatusFail, got %q: %s", result.Status, result.Detail)
	}
}

func TestProbeRESTServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	prober := NewProber(
		WithHTTPClient(srv.Client()),
		WithTimeout(5*time.Second),
	)
	result := prober.probeREST(context.Background(), CatalogEntry{
		ID:             "error-rest",
		DisplayName:    "Error REST",
		Transport:      TransportREST,
		HealthEndpoint: "/health",
	}, srv.URL)

	if result.Status != StatusFail {
		t.Errorf("expected StatusFail, got %q: %s", result.Status, result.Detail)
	}
}

func TestProbeRESTNoEndpointConfigured(t *testing.T) {
	t.Parallel()

	prober := NewProber()
	result := prober.probeREST(context.Background(), CatalogEntry{
		ID:             "no-endpoint",
		DisplayName:    "No Endpoint",
		Transport:      TransportREST,
		HealthEndpoint: "/health",
	}, "") // empty base URL

	if result.Status != StatusFail {
		t.Errorf("expected StatusFail for empty endpoint, got %q: %s", result.Status, result.Detail)
	}
}

func TestProbeRESTNoHealthEndpointPath(t *testing.T) {
	t.Parallel()

	prober := NewProber()
	result := prober.probeREST(context.Background(), CatalogEntry{
		ID:             "no-healthcheck",
		DisplayName:    "No Health Check",
		Transport:      TransportREST,
		HealthEndpoint: "", // no health check path configured
	}, "https://api.example.com")

	if result.Status != StatusPass {
		t.Errorf("expected StatusPass when no health endpoint, got %q: %s", result.Status, result.Detail)
	}
}

func TestProbeAllIteratesOverCatalog(t *testing.T) {
	t.Parallel()

	prober := NewProber(WithTimeout(2 * time.Second))
	results := prober.ProbeAll(context.Background(), nil, nil)

	catalogSize := len(AllCatalogEntries())
	if len(results) != catalogSize {
		t.Fatalf("expected %d results, got %d", catalogSize, len(results))
	}

	// Verify every result has a valid ID from catalog.
	for _, result := range results {
		if _, ok := LookupCatalog(result.ID); !ok {
			t.Errorf("result ID %q not found in catalog", result.ID)
		}
		if result.DisplayName == "" {
			t.Errorf("result for %q has empty DisplayName", result.ID)
		}
		if result.Transport == "" {
			t.Errorf("result for %q has empty Transport", result.ID)
		}
		// Status must be one of the known values.
		switch result.Status {
		case StatusPass, StatusWarn, StatusFail:
			// valid
		default:
			t.Errorf("result for %q has unknown status %q", result.ID, result.Status)
		}
	}
}

func TestProbeAllWithOverrides(t *testing.T) {
	t.Parallel()

	// Provide a REST endpoint for Ollama using a test server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewProber(
		WithHTTPClient(srv.Client()),
		WithTimeout(2*time.Second),
	)

	restEndpoints := map[ID]string{
		Ollama: srv.URL,
	}

	results := prober.ProbeAll(context.Background(), nil, restEndpoints)

	var ollamaResult *ProbeResult
	for i, r := range results {
		if r.ID == Ollama {
			ollamaResult = &results[i]
			break
		}
	}
	if ollamaResult == nil {
		t.Fatal("Ollama not found in results")
	}
	if ollamaResult.Status != StatusPass {
		t.Errorf("expected Ollama StatusPass with test server, got %q: %s", ollamaResult.Status, ollamaResult.Detail)
	}
}

func TestProbeAllUsesDefaultBaseURL(t *testing.T) {
	t.Parallel()

	// With no overrides, REST agents should use their DefaultBaseURL
	// rather than reporting "no endpoint configured".
	prober := NewProber(WithTimeout(2 * time.Second))
	results := prober.ProbeAll(context.Background(), nil, nil)

	for _, r := range results {
		entry, _ := LookupCatalog(r.ID)
		if entry.Transport == TransportREST && entry.DefaultBaseURL != "" {
			// Should NOT be "no endpoint configured" when a default exists.
			if r.Detail == "no endpoint configured" {
				t.Errorf("REST agent %q should use DefaultBaseURL, got 'no endpoint configured'", r.ID)
			}
		}
	}
}

func TestProbeOneKnownAgent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prober := NewProber(
		WithHTTPClient(srv.Client()),
		WithTimeout(2*time.Second),
	)
	result, err := prober.ProbeOne(context.Background(), Ollama, "", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusPass {
		t.Errorf("expected StatusPass, got %q: %s", result.Status, result.Detail)
	}
}

func TestProbeOneUnknownAgent(t *testing.T) {
	t.Parallel()

	prober := NewProber()
	_, err := prober.ProbeOne(context.Background(), "unknown-agent", "", "")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestProbeResultHealthy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status  ProbeStatus
		healthy bool
	}{
		{StatusPass, true},
		{StatusWarn, true},
		{StatusFail, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			r := ProbeResult{Status: tc.status}
			if r.Healthy() != tc.healthy {
				t.Errorf("Healthy() = %v, want %v", r.Healthy(), tc.healthy)
			}
		})
	}
}

func TestParseVersionVariousFormats(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected string
	}{
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"V1.2.3", "1.2.3"},
		{"claude 1.0.30", "1.0.30"},
		{"codex version 0.1.2", "0.1.2"},
		{"v1.2.3\nsome extra output", "1.2.3"},
		{"  v1.2.3  ", "1.2.3"},
		{"", ""},
		{"  ", ""},
		{"some random text", ""},
		{"tool v2.0.0-beta.1", "2.0.0-beta.1"},
		// Node.js deprecation warning before version (Gemini CLI).
		{"(node:12345) [DEP0040] DeprecationWarning: The 'punycode' module is deprecated\n0.3.5", "0.3.5"},
		// Version buried in multi-line noise.
		{"Loading...\nInitializing...\nv1.5.0\nDone.", "1.5.0"},
		// ANSI escape codes in output (OpenCode).
		{"\x1b[91m\x1b[1mError: \x1b[0mFailed\n1.0.0", "1.0.0"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			got := parseVersion(tc.input)
			if got != tc.expected {
				t.Errorf("parseVersion(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected string
	}{
		{"no escapes", "no escapes"},
		{"\x1b[91m\x1b[1mError: \x1b[0mFailed", "Error: Failed"},
		{"\x1b[0;32m1.0.0\x1b[0m", "1.0.0"},
		{"", ""},
		{"\x1b[31mred\x1b[0m and \x1b[32mgreen\x1b[0m", "red and green"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			got := stripANSI(tc.input)
			if got != tc.expected {
				t.Errorf("stripANSI(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestSummarizeNetError(t *testing.T) {
	t.Parallel()

	got := summarizeNetError(fmt.Errorf("Get http://localhost:1/health: dial tcp 127.0.0.1:1: connection refused"))
	if got != "connection refused" {
		t.Errorf("expected 'connection refused', got %q", got)
	}
}

func TestProberOptionsApplied(t *testing.T) {
	t.Parallel()

	client := &http.Client{Timeout: 42 * time.Second}
	p := NewProber(
		WithHTTPClient(client),
		WithTimeout(30*time.Second),
	)
	if p.httpClient != client {
		t.Error("WithHTTPClient option not applied")
	}
	if p.timeout != 30*time.Second {
		t.Errorf("WithTimeout option not applied: got %v", p.timeout)
	}
}
