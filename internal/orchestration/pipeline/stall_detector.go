package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"strings"
)

var (
	reTimestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}t\d{2}:\d{2}:\d{2}(?:\.\d+)?z?`)
	reUUID      = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
	rePath      = regexp.MustCompile(`(?:[A-Za-z]:\\[^\s]+|/[^\s]+)`)
	reSpace     = regexp.MustCompile(`\s+`)
)

// StallDetector keeps fingerprints per task+phase and flags repetitive outputs.
type StallDetector struct {
	window    int
	threshold float64
	history   map[string][]string
}

func NewStallDetector(window int, threshold float64) *StallDetector {
	if window < 2 {
		window = 3
	}
	if threshold <= 0 || threshold > 1 {
		threshold = 0.67
	}
	return &StallDetector{
		window:    window,
		threshold: threshold,
		history:   make(map[string][]string),
	}
}

func (d *StallDetector) Observe(taskID, phase, output string) (bool, float64) {
	if d == nil {
		return false, 0
	}
	key := strings.TrimSpace(taskID) + ":" + strings.TrimSpace(phase)
	fp := d.fingerprint(output)
	window := append(d.history[key], fp)
	if len(window) > d.window {
		window = window[len(window)-d.window:]
	}
	d.history[key] = window
	if len(window) < d.window {
		return false, 0
	}

	counts := make(map[string]int)
	maxCount := 0
	for _, item := range window {
		counts[item]++
		if counts[item] > maxCount {
			maxCount = counts[item]
		}
	}
	similarity := float64(maxCount) / float64(len(window))
	similarity = math.Round(similarity*100) / 100
	return similarity >= d.threshold, similarity
}

func (d *StallDetector) fingerprint(output string) string {
	normalized := normalizeStallOutput(output)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func normalizeStallOutput(value string) string {
	value = strings.ToLower(value)
	value = reTimestamp.ReplaceAllString(value, "<ts>")
	value = reUUID.ReplaceAllString(value, "<uuid>")
	value = rePath.ReplaceAllString(value, "<path>")
	value = reSpace.ReplaceAllString(strings.TrimSpace(value), " ")
	return value
}
