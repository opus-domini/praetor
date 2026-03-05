package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type gateStatus string

const (
	gateStatusPass    gateStatus = "PASS"
	gateStatusFail    gateStatus = "FAIL"
	gateStatusMissing gateStatus = "MISSING"
	gateStatusError   gateStatus = "ERROR"
)

var supportedGateNames = map[string]struct{}{
	"tests":     {},
	"lint":      {},
	"standards": {},
}

// GateResult captures one host-executed gate evaluation result.
type GateResult struct {
	Name      string
	Status    gateStatus
	Required  bool
	Detail    string
	Command   string
	DurationS float64
}

// GateRunner executes quality gates in the host process.
type GateRunner interface {
	Run(ctx context.Context, workdir string, required, optional []string, timeout time.Duration) []GateResult
}

type gateExecutor func(ctx context.Context, workdir, command string) (string, error)

// HostGateRunner executes gate commands via the local shell.
type HostGateRunner struct {
	commands map[string]string
	execFn   gateExecutor
}

func NewHostGateRunner(commands map[string]string) *HostGateRunner {
	copied := make(map[string]string, len(commands))
	for key, value := range commands {
		copied[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return &HostGateRunner{
		commands: copied,
		execFn:   defaultGateExec,
	}
}

func (r *HostGateRunner) Run(ctx context.Context, workdir string, required, optional []string, timeout time.Duration) []GateResult {
	results := make([]GateResult, 0, len(required)+len(optional))
	for _, gate := range required {
		results = append(results, r.runOne(ctx, workdir, gate, true, timeout))
	}
	for _, gate := range optional {
		results = append(results, r.runOne(ctx, workdir, gate, false, timeout))
	}
	return results
}

func (r *HostGateRunner) runOne(ctx context.Context, workdir, gate string, required bool, timeout time.Duration) GateResult {
	name := strings.ToLower(strings.TrimSpace(gate))
	if name == "" {
		return GateResult{}
	}
	result := GateResult{
		Name:     name,
		Required: required,
	}

	command := strings.TrimSpace(r.commands[name])
	result.Command = command

	if command == "" {
		if _, ok := supportedGateNames[name]; !ok {
			result.Status = gateStatusMissing
			result.Detail = fmt.Sprintf("unsupported gate %q", name)
			return result
		}
		result.Status = gateStatusMissing
		result.Detail = "gate command is not configured"
		return result
	}

	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	start := time.Now()
	output, err := r.execFn(runCtx, workdir, command)
	result.DurationS = time.Since(start).Seconds()
	result.Detail = trimGateOutput(output)

	if err == nil {
		result.Status = gateStatusPass
		return result
	}
	if runCtx.Err() == context.DeadlineExceeded {
		result.Status = gateStatusError
		if result.Detail == "" {
			result.Detail = "gate timeout"
		}
		return result
	}
	result.Status = gateStatusFail
	if result.Detail == "" {
		result.Detail = err.Error()
	}
	return result
}

func defaultGateExec(ctx context.Context, workdir, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.Dir = workdir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func trimGateOutput(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
