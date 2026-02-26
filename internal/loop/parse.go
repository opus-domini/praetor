package loop

import "github.com/opus-domini/praetor/internal/domain"

// ParseExecutorResult delegates to domain.ParseExecutorResult.
func ParseExecutorResult(output string) ExecutorResult {
	return domain.ParseExecutorResult(output)
}

// ParseReviewDecision delegates to domain.ParseReviewDecision.
func ParseReviewDecision(output string) ReviewDecision {
	return domain.ParseReviewDecision(output)
}
