package loop

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

func (r *Renderer) Header(title string) {
	_, _ = fmt.Fprintf(r.out, "\n%s=== %s ===%s\n", r.c("1;36"), strings.TrimSpace(title), r.reset())
}

func (r *Renderer) KV(label, value string) {
	_, _ = fmt.Fprintf(r.out, "%s%-12s%s %s\n", r.c("2"), label, r.reset(), value)
}

func (r *Renderer) Task(progress, label, title string) {
	_, _ = fmt.Fprintf(r.out, "\n%s[%s] %s%s %s\n", r.c("1;34"), progress, label, r.reset(), title)
}

func (r *Renderer) Phase(phase, agent, detail string) {
	_, _ = fmt.Fprintf(r.out, "  %s%-8s%s (%s)%s %s\n", r.c("35"), phase, r.reset(), agent, r.reset(), detail)
}

func (r *Renderer) Info(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[info]%s %s\n", r.c("34"), r.reset(), message)
}

func (r *Renderer) Success(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[ok]%s %s\n", r.c("32"), r.reset(), message)
}

func (r *Renderer) Warn(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[warn]%s %s\n", r.c("33"), r.reset(), message)
}

func (r *Renderer) Error(message string) {
	_, _ = fmt.Fprintf(r.out, "  %s[err]%s %s\n", r.c("31"), r.reset(), message)
}

func (r *Renderer) Dim(message string) {
	_, _ = fmt.Fprintf(r.out, "%s%s%s\n", r.c("2"), message, r.reset())
}

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

	type fileDescriptor interface {
		Fd() uintptr
	}
	f, ok := out.(fileDescriptor)
	if !ok {
		return false
	}
	return isTerminalFD(f.Fd())
}

func isTerminalFD(fd uintptr) bool {
	file := os.NewFile(fd, "/dev/fd")
	if file == nil {
		return false
	}
	if info, err := file.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
		return true
	}
	return false
}
