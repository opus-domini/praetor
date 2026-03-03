package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Renderer prints structured, colored terminal output.
type Renderer struct {
	out   io.Writer
	color bool
}

func NewRenderer(out io.Writer, noColor bool) *Renderer {
	return &Renderer{
		out:   out,
		color: detectColor(out, noColor),
	}
}

// Banner prints a bold header line with project context.
func (r *Renderer) Banner(tool, message string) {
	_, _ = fmt.Fprintf(r.out, "\n  %s%s%s — %s\n", r.c("1;36"), tool, r.reset(), message)
}

// Header prints a section header.
func (r *Renderer) Header(title string) {
	_, _ = fmt.Fprintf(r.out, "\n%s=== %s ===%s\n", r.c("1;36"), strings.TrimSpace(title), r.reset())
}

// Step prints a numbered step header (e.g., "[1/3] Agent Commands").
func (r *Renderer) Step(current, total int, title string) {
	_, _ = fmt.Fprintf(r.out, "\n  %s[%d/%d]%s %s%s%s\n",
		r.c("1;34"), current, total, r.reset(),
		r.c("1"), title, r.reset())
}

// KV prints a key-value pair with dim label.
func (r *Renderer) KV(label, value string) {
	_, _ = fmt.Fprintf(r.out, "%s%-12s%s %s\n", r.c("2"), label, r.reset(), value)
}

// ConfigKV prints a config key-value pair with source annotation.
func (r *Renderer) ConfigKV(key, value, source string) {
	_, _ = fmt.Fprintf(r.out, "  %-26s = %-30s %s(%s)%s\n", key, value, r.c("2"), source, r.reset())
}

// Task prints a task progress header for plan execution.
func (r *Renderer) Task(progress, label, title string) {
	_, _ = fmt.Fprintf(r.out, "\n%s[%s] %s%s %s\n", r.c("1;34"), progress, label, r.reset(), title)
}

// Phase prints an execution phase line (executor, reviewer, hook).
func (r *Renderer) Phase(phase, agent, detail string) {
	_, _ = fmt.Fprintf(r.out, "  %s%-8s%s (%s) %s\n", r.c("35"), phase, r.reset(), agent, detail)
}

// Info prints an informational message with [info] tag.
func (r *Renderer) Info(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[info]%s %s\n", r.c("34"), r.reset(), message)
}

// Success prints a success message with [ok] tag.
func (r *Renderer) Success(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[ok]%s %s\n", r.c("32"), r.reset(), message)
}

// Warn prints a warning message with [warn] tag.
func (r *Renderer) Warn(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[warn]%s %s\n", r.c("33"), r.reset(), message)
}

// Error prints an error message with [err] tag.
func (r *Renderer) Error(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[err]%s %s\n", r.c("31"), r.reset(), message)
}

// Dim prints a dim (muted) line.
func (r *Renderer) Dim(message string) {
	_, _ = fmt.Fprintf(r.out, "%s%s%s\n", r.c("2"), message, r.reset())
}

// Done prints a bold success completion message.
func (r *Renderer) Done(message string) {
	_, _ = fmt.Fprintf(r.out, "\n  %s%s%s\n", r.c("1;32"), message, r.reset())
}

// Hint prints a dim indented line for secondary information.
func (r *Renderer) Hint(message string) {
	_, _ = fmt.Fprintf(r.out, "    %s%s%s\n", r.c("2"), message, r.reset())
}

// Blank prints an empty line.
func (r *Renderer) Blank() {
	_, _ = fmt.Fprintln(r.out)
}

// CheckItem prints a checklist item like "  [x] ID: Title" with colored mark.
// variant: "done", "fail", "active", "pending".
func (r *Renderer) CheckItem(variant, id, title string) {
	var mark, color string
	switch variant {
	case "done":
		mark, color = "x", "32"
	case "fail":
		mark, color = "!", "31"
	case "active":
		mark, color = ">", "33"
	default:
		mark, color = " ", "2"
	}
	_, _ = fmt.Fprintf(r.out, "  [%s%s%s] %s: %s\n", r.c(color), mark, r.reset(), id, title)
}

// Summary prints the run summary line.
func (r *Renderer) Summary(done, rejected, iterations int, totalCostUSD float64, totalDuration time.Duration) {
	costStr := ""
	if totalCostUSD > 0 {
		costStr = fmt.Sprintf(" cost=$%.4f", totalCostUSD)
	}
	durationStr := ""
	if totalDuration > 0 {
		durationStr = fmt.Sprintf(" duration=%s", totalDuration.Truncate(time.Second))
	}
	_, _ = fmt.Fprintf(r.out, "\n%sRun summary%s done=%d rejected=%d iterations=%d%s%s\n",
		r.c("1"), r.reset(), done, rejected, iterations, costStr, durationStr)
}

func (r *Renderer) c(code string) string {
	if !r.color {
		return ""
	}
	return "\033[" + code + "m"
}

func (r *Renderer) reset() string {
	if !r.color {
		return ""
	}
	return "\033[0m"
}

func detectColor(out io.Writer, noColor bool) bool {
	if noColor {
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	if strings.TrimSpace(os.Getenv("TERM")) == "dumb" {
		return false
	}
	switch out {
	case os.Stdout, os.Stderr:
		return true
	default:
		return false
	}
}
