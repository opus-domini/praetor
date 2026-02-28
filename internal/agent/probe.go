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
	StatusOK          ProbeStatus = "ok"
	StatusNotFound    ProbeStatus = "not_found"
	StatusError       ProbeStatus = "error"
	StatusUnreachable ProbeStatus = "unreachable"
)

// ProbeResult holds the outcome of a health check for one agent.
type ProbeResult struct {
	ID          ID
	DisplayName string
	Transport   Transport
	Status      ProbeStatus
	Version     string // parsed version string, empty if unavailable
	Path        string // resolved binary path (CLI) or base URL (REST)
	Detail      string // human-readable status detail or error message
}

// Healthy reports whether the probe found the agent operational.
func (r ProbeResult) Healthy() bool {
	return r.Status == StatusOK
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
				Status:      StatusError,
				Detail:      fmt.Sprintf("unknown transport %q", entry.Transport),
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
			Status:      StatusError,
			Detail:      fmt.Sprintf("unknown transport %q", entry.Transport),
		}, nil
	}
}

func (p *Prober) probeCLI(ctx context.Context, entry CatalogEntry, binary string) ProbeResult {
	result := ProbeResult{
		ID:          entry.ID,
		DisplayName: entry.DisplayName,
		Transport:   entry.Transport,
	}

	resolvedPath, err := exec.LookPath(binary)
	if err != nil {
		result.Status = StatusNotFound
		result.Detail = fmt.Sprintf("%s not found in PATH", binary)
		return result
	}
	result.Path = resolvedPath

	if len(entry.VersionArgs) == 0 {
		result.Status = StatusOK
		result.Detail = "binary found (version detection not configured)"
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
		// Binary exists but version command failed — still usable.
		result.Status = StatusOK
		result.Detail = "binary found (version command failed: " + strings.TrimSpace(err.Error()) + ")"
		return result
	}

	// Parse stdout first (avoids Node.js warnings on stderr).
	// If stdout has no version, try stderr (some tools like opencode write there).
	version := parseVersion(stripANSI(strings.TrimSpace(stdoutBuf.String())))
	if version == "" {
		version = parseVersion(stripANSI(strings.TrimSpace(stderrBuf.String())))
	}
	result.Version = version
	result.Status = StatusOK
	if version != "" {
		result.Detail = "v" + version
	} else {
		result.Detail = "binary found"
	}
	return result
}

func (p *Prober) probeREST(ctx context.Context, entry CatalogEntry, baseURL string) ProbeResult {
	result := ProbeResult{
		ID:          entry.ID,
		DisplayName: entry.DisplayName,
		Transport:   entry.Transport,
	}

	if baseURL == "" {
		result.Status = StatusError
		result.Detail = "no endpoint configured"
		return result
	}
	result.Path = baseURL

	if entry.HealthEndpoint == "" {
		result.Status = StatusOK
		result.Detail = "endpoint configured (no health check path)"
		return result
	}

	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + entry.HealthEndpoint
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		result.Status = StatusError
		result.Detail = "invalid endpoint URL: " + err.Error()
		return result
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		result.Status = StatusUnreachable
		result.Detail = "endpoint unreachable: " + summarizeNetError(err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = StatusOK
		result.Detail = fmt.Sprintf("reachable (HTTP %d)", resp.StatusCode)
	} else {
		result.Status = StatusError
		result.Detail = fmt.Sprintf("endpoint returned HTTP %d", resp.StatusCode)
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
