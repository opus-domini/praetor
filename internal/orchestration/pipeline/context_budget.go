package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/agent/middleware"
)

const (
	promptPhaseExecute = "execute"
	promptPhaseReview  = "review"
)

// ContextBudgetManager controls prompt sizes by phase.
type ContextBudgetManager struct {
	executeChars int
	reviewChars  int
}

func NewContextBudgetManager(executeChars, reviewChars int) *ContextBudgetManager {
	if executeChars <= 0 {
		executeChars = 120000
	}
	if reviewChars <= 0 {
		reviewChars = 80000
	}
	return &ContextBudgetManager{executeChars: executeChars, reviewChars: reviewChars}
}

func (m *ContextBudgetManager) Budget(phase string) int {
	if m == nil {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case promptPhaseExecute:
		return m.executeChars
	case promptPhaseReview:
		return m.reviewChars
	default:
		return 0
	}
}

func (m *ContextBudgetManager) Check(phase, prompt string) bool {
	budget := m.Budget(phase)
	if budget <= 0 {
		return true
	}
	return len(prompt) <= budget
}

func (m *ContextBudgetManager) EstimateTokens(prompt string) int {
	if strings.TrimSpace(prompt) == "" {
		return 0
	}
	return len(prompt) / 4
}

func (m *ContextBudgetManager) TruncateExecuteFeedback(promptWithoutFeedback, feedback string) (string, []string) {
	budget := m.Budget(promptPhaseExecute)
	if budget <= 0 {
		return feedback, nil
	}
	remaining := budget - len(promptWithoutFeedback)
	if remaining <= 0 {
		return "", []string{"feedback"}
	}
	if len(feedback) <= remaining {
		return feedback, nil
	}
	return truncateTail(feedback, remaining), []string{"feedback"}
}

func (m *ContextBudgetManager) TruncateReviewSections(promptOverhead, executorOutput, gitDiff string) (string, string, []string) {
	budget := m.Budget(promptPhaseReview)
	if budget <= 0 {
		return executorOutput, gitDiff, nil
	}

	sectionsTruncated := make([]string, 0)
	remaining := budget - len(promptOverhead)
	if remaining <= 0 {
		return "", "", []string{"executor_output", "git_diff"}
	}

	execOut := executorOutput
	diff := gitDiff
	current := len(execOut) + len(diff)
	if current <= remaining {
		return execOut, diff, nil
	}

	// Priority 1: truncate executor output first.
	maxExec := remaining * 2 / 3
	if maxExec < 0 {
		maxExec = 0
	}
	if len(execOut) > maxExec {
		execOut = truncateTail(execOut, maxExec)
		sectionsTruncated = append(sectionsTruncated, "executor_output")
	}

	current = len(execOut) + len(diff)
	if current > remaining {
		maxDiff := remaining - len(execOut)
		if maxDiff < 0 {
			maxDiff = 0
		}
		if len(diff) > maxDiff {
			diff = truncateTail(diff, maxDiff)
			sectionsTruncated = append(sectionsTruncated, "git_diff")
		}
	}

	current = len(execOut) + len(diff)
	if current > remaining {
		// Final guard: shrink executor output again if needed.
		maxExec = remaining - len(diff)
		if maxExec < 0 {
			maxExec = 0
		}
		execOut = truncateTail(execOut, maxExec)
		if !containsString(sectionsTruncated, "executor_output") {
			sectionsTruncated = append(sectionsTruncated, "executor_output")
		}
	}

	return execOut, diff, sectionsTruncated
}

func truncateTail(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if maxChars <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxChars {
		return value
	}
	return strings.TrimSpace(value[len(value)-maxChars:])
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

type promptPerformanceEntry struct {
	Iteration         int      `json:"iteration"`
	Phase             string   `json:"phase"`
	PromptChars       int      `json:"prompt_chars"`
	EstimatedTokens   int      `json:"estimated_tokens"`
	SectionsTruncated []string `json:"sections_truncated,omitempty"`
}

func (run *activeRun) appendPerformanceEntry(phase, prompt string, truncated []string) error {
	if run == nil || run.budgetManager == nil || strings.TrimSpace(run.performancePath) == "" {
		return nil
	}
	entry := promptPerformanceEntry{
		Iteration:         run.stats.Iterations,
		Phase:             strings.TrimSpace(phase),
		PromptChars:       len(prompt),
		EstimatedTokens:   run.budgetManager.EstimateTokens(prompt),
		SectionsTruncated: truncated,
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode prompt performance entry: %w", err)
	}
	encoded = append(encoded, '\n')

	f, err := os.OpenFile(run.performancePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open performance log: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(encoded); err != nil {
		return fmt.Errorf("write performance log: %w", err)
	}
	if run.eventSink != nil && len(truncated) > 0 {
		run.eventSink.Emit(middleware.ExecutionEvent{
			SchemaVersion: 1,
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Type:          middleware.EventBudgetWarning,
			EventType:     string(middleware.EventBudgetWarning),
			RunID:         run.runID,
			Phase:         strings.TrimSpace(phase),
			Action:        "truncated",
			Message:       strings.Join(truncated, ","),
			Data: map[string]any{
				"phase":              strings.TrimSpace(phase),
				"prompt_chars":       len(prompt),
				"estimated_tokens":   run.budgetManager.EstimateTokens(prompt),
				"sections_truncated": truncated,
			},
		})
	}
	return nil
}
