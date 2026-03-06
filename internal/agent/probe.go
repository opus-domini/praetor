package agent

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ProbeStatus classifies the health of an agent backend.
type ProbeStatus string

const (
	StatusPass ProbeStatus = "pass"
	StatusWarn ProbeStatus = "warn"
	StatusFail ProbeStatus = "fail"
)

// HealthCheck describes one structured environment check.
type HealthCheck struct {
	Code    string `json:"code"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// ProbeResult holds the outcome of a health check for one agent.
type ProbeResult struct {
	ID          ID
	DisplayName string
	Transport   Transport
	Status      ProbeStatus
	Version     string // parsed version string, empty if unavailable
	Path        string // resolved binary path (CLI) or base URL (REST)
	Detail      string // human-readable status detail or error message
	Checks      []HealthCheck
}

// Healthy reports whether the probe found the agent operational.
func (r ProbeResult) Healthy() bool {
	return r.Status != StatusFail
}

// Prober runs health checks against agent backends.
type Prober struct {
	httpClient *http.Client
	timeout    time.Duration
}

// ProberOption configures a Prober.
type ProberOption func(*Prober)

// WithHTTPClient sets the HTTP client for REST probes.
func WithHTTPClient(c *http.Client) ProberOption {
	return func(p *Prober) { p.httpClient = c }
}

// WithTimeout sets the per-probe timeout.
func WithTimeout(d time.Duration) ProberOption {
	return func(p *Prober) { p.timeout = d }
}

// NewProber creates a health prober with the given options.
func NewProber(opts ...ProberOption) *Prober {
	p := &Prober{
		httpClient: &http.Client{Timeout: 5 * time.Second},
		timeout:    10 * time.Second,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ProbeAll runs health checks for all catalog entries.
// binaryOverrides maps agent ID to custom binary path (from config).
// restEndpoints maps agent ID to base URL (from config).
func (p *Prober) ProbeAll(ctx context.Context, binaryOverrides map[ID]string, restEndpoints map[ID]string) []ProbeResult {
	entries := AllCatalogEntries()
	results := make([]ProbeResult, 0, len(entries))
	for _, entry := range entries {
		var result ProbeResult
		switch entry.Transport {
		case TransportCLI:
			binary := entry.Binary
			if override, ok := binaryOverrides[entry.ID]; ok && override != "" {
				binary = override
			}
			result = p.probeCLI(ctx, entry, binary)
		case TransportREST:
			baseURL := entry.DefaultBaseURL
			if endpoint, ok := restEndpoints[entry.ID]; ok && endpoint != "" {
				baseURL = endpoint
			}
			result = p.probeREST(ctx, entry, baseURL)
		default:
			result = ProbeResult{
				ID:          entry.ID,
				DisplayName: entry.DisplayName,
				Transport:   entry.Transport,
				Status:      StatusFail,
				Detail:      fmt.Sprintf("unknown transport %q", entry.Transport),
				Checks: []HealthCheck{{
					Code:    "unknown_transport",
					Level:   "error",
					Message: "Unknown transport",
					Detail:  fmt.Sprintf("transport %q is not supported", entry.Transport),
				}},
			}
		}
		results = append(results, result)
	}
	return results
}

// ProbeOne runs a health check for a single agent.
func (p *Prober) ProbeOne(ctx context.Context, id ID, binaryOverride string, restEndpoint string) (ProbeResult, error) {
	entry, ok := LookupCatalog(id)
	if !ok {
		return ProbeResult{}, fmt.Errorf("unknown agent %q", id)
	}

	switch entry.Transport {
	case TransportCLI:
		binary := entry.Binary
		if binaryOverride != "" {
			binary = binaryOverride
		}
		return p.probeCLI(ctx, entry, binary), nil
	case TransportREST:
		endpoint := restEndpoint
		if endpoint == "" {
			endpoint = entry.DefaultBaseURL
		}
		return p.probeREST(ctx, entry, endpoint), nil
	default:
		return ProbeResult{
			ID:          entry.ID,
			DisplayName: entry.DisplayName,
			Transport:   entry.Transport,
			Status:      StatusFail,
			Detail:      fmt.Sprintf("unknown transport %q", entry.Transport),
			Checks: []HealthCheck{{
				Code:    "unknown_transport",
				Level:   "error",
				Message: "Unknown transport",
				Detail:  fmt.Sprintf("transport %q is not supported", entry.Transport),
			}},
		}, nil
	}
}

func addCheck(result *ProbeResult, code, level, message, detail, hint string) {
	if result == nil {
		return
	}
	result.Checks = append(result.Checks, HealthCheck{
		Code:    code,
		Level:   level,
		Message: message,
		Detail:  strings.TrimSpace(detail),
		Hint:    strings.TrimSpace(hint),
	})
	result.Status = aggregateProbeStatus(result.Checks)
	if strings.TrimSpace(detail) != "" {
		result.Detail = strings.TrimSpace(detail)
	} else {
		result.Detail = strings.TrimSpace(message)
	}
}

func aggregateProbeStatus(checks []HealthCheck) ProbeStatus {
	status := StatusPass
	for _, check := range checks {
		switch strings.ToLower(strings.TrimSpace(check.Level)) {
		case "error":
			return StatusFail
		case "warn":
			status = StatusWarn
		}
	}
	return status
}

func (p *Prober) probeCLI(ctx context.Context, entry CatalogEntry, binary string) ProbeResult {
	result := ProbeResult{
		ID:          entry.ID,
		DisplayName: entry.DisplayName,
		Transport:   entry.Transport,
		Status:      StatusPass,
	}

	resolvedPath, err := exec.LookPath(binary)
	if err != nil {
		addCheck(&result, "binary_missing", "error", "Binary not found", fmt.Sprintf("%s not found in PATH", binary), entry.InstallHint)
		return result
	}
	result.Path = resolvedPath
	addCheck(&result, "binary_found", "info", "Binary found", resolvedPath, "")

	if len(entry.VersionArgs) == 0 {
		addCheck(&result, "version_skipped", "info", "Version check skipped", "version detection is not configured for this adapter", "")
		return result
	}

	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, resolvedPath, entry.VersionArgs...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err = cmd.Run()
	if err != nil {
		addCheck(&result, "version_probe_failed", "warn", "Version command failed", strings.TrimSpace(err.Error()), "Check whether the installed CLI supports --version or update it.")
		return result
	}

	// Parse stdout first (avoids Node.js warnings on stderr).
	// If stdout has no version, try stderr (some tools like opencode write there).
	version := parseVersion(stripANSI(strings.TrimSpace(stdoutBuf.String())))
	if version == "" {
		version = parseVersion(stripANSI(strings.TrimSpace(stderrBuf.String())))
	}
	result.Version = version
	if version != "" {
		addCheck(&result, "version_parsed", "info", "Version detected", "v"+version, "")
	} else {
		addCheck(&result, "version_missing", "warn", "Binary found but version could not be parsed", strings.TrimSpace(stdoutBuf.String()), "Run the binary manually and confirm it prints a semantic version.")
	}
	return result
}

func (p *Prober) probeREST(ctx context.Context, entry CatalogEntry, baseURL string) ProbeResult {
	result := ProbeResult{
		ID:          entry.ID,
		DisplayName: entry.DisplayName,
		Transport:   entry.Transport,
		Status:      StatusPass,
	}

	if baseURL == "" {
		addCheck(&result, "endpoint_missing", "error", "Endpoint is not configured", "no endpoint configured", entry.InstallHint)
		return result
	}
	result.Path = baseURL
	addCheck(&result, "endpoint_configured", "info", "Endpoint configured", baseURL, "")

	if entry.HealthEndpoint == "" {
		addCheck(&result, "healthcheck_skipped", "info", "Health check skipped", "adapter has no dedicated health endpoint", "")
		return result
	}

	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + entry.HealthEndpoint
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		addCheck(&result, "endpoint_invalid", "error", "Endpoint URL is invalid", err.Error(), entry.InstallHint)
		return result
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		addCheck(&result, "endpoint_unreachable", "error", "Health endpoint unreachable", summarizeNetError(err), restHint(entry))
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		addCheck(&result, "endpoint_reachable", "info", "Health endpoint reachable", fmt.Sprintf("HTTP %d", resp.StatusCode), "")
	} else {
		addCheck(&result, "endpoint_error", "error", "Health endpoint returned an error", fmt.Sprintf("HTTP %d", resp.StatusCode), restHint(entry))
	}
	return result
}

// ansiPattern matches ANSI escape sequences (colors, cursor movement, etc.).
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// semverPattern matches a semantic version (possibly with pre-release suffix).
var semverPattern = regexp.MustCompile(`\d+\.\d+[\.\d]*[-\w.]*`)

// stripANSI removes ANSI escape sequences from CLI output.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// parseVersion extracts a version string from CLI output.
// It handles common formats like "v1.2.3", "claude 1.2.3", "1.2.3\n",
// and is resilient to ANSI escape codes and multi-line noise.
func parseVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Scan all lines for the first semver-like match. This handles
	// cases where warnings or noise precede the actual version line.
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try to find a semver pattern anywhere in the line.
		if m := semverPattern.FindString(line); m != "" {
			return m
		}
	}

	// No semver-like pattern found anywhere.
	return ""
}

// summarizeNetError extracts a concise error message from a network error.
func summarizeNetError(err error) string {
	msg := err.Error()
	// Trim verbose wrapped context.
	if idx := strings.LastIndex(msg, ": "); idx >= 0 {
		short := msg[idx+2:]
		if short != "" {
			return short
		}
	}
	return msg
}

func restHint(entry CatalogEntry) string {
	switch entry.ID {
	case Ollama:
		return "Start Ollama with: ollama serve"
	case LMStudio:
		return "Start the LM Studio local server and verify the configured port."
	case OpenRouter:
		return "Verify the endpoint URL and that the required API key is configured."
	default:
		return entry.InstallHint
	}
}
