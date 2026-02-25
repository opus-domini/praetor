package loop

import (
	"fmt"
	"strings"
)

func buildExecutorSystemPrompt() string {
	return strings.TrimSpace(`You are an autonomous executor agent in an automated task pipeline.
Your job is to complete exactly one task.

Rules:
- Only implement what the task requests.
- Avoid unrelated modifications.
- Run the smallest relevant tests to validate changes.
- End the response with the required RESULT block.

Required result format:
RESULT: PASS
SUMMARY: <brief summary>
TESTS: <commands and outcomes>

If not completed, use RESULT: FAIL and explain why.`)
}

func buildExecutorTaskPrompt(planFile string, taskIndex int, task StateTask, previousFeedback string, retryCount int, planTitle, progress, workdir string) string {
	var b strings.Builder

	if previousFeedback != "" && retryCount > 0 {
		fmt.Fprintf(&b, "RETRY CONTEXT (attempt %d)\n", retryCount+1)
		fmt.Fprintf(&b, "%s\n\n", previousFeedback)
	}

	fmt.Fprintf(&b, "TASK\n")
	fmt.Fprintf(&b, "  Title: %s\n", task.Title)
	fmt.Fprintf(&b, "  ID: %s\n", task.ID)
	fmt.Fprintf(&b, "  Index: %d\n", taskIndex)
	if len(task.DependsOn) > 0 {
		fmt.Fprintf(&b, "  Depends on: %s\n", strings.Join(task.DependsOn, ","))
	}

	fmt.Fprintf(&b, "\nPLAN\n")
	fmt.Fprintf(&b, "  File: %s\n", planFile)
	if strings.TrimSpace(planTitle) != "" {
		fmt.Fprintf(&b, "  Title: %s\n", strings.TrimSpace(planTitle))
	}
	if strings.TrimSpace(progress) != "" {
		fmt.Fprintf(&b, "  Progress: %s\n", strings.TrimSpace(progress))
	}

	fmt.Fprintf(&b, "\nWORKDIR\n  %s\n", workdir)

	if task.Description != "" {
		fmt.Fprintf(&b, "\nDESCRIPTION\n%s\n", task.Description)
	}
	if task.Criteria != "" {
		fmt.Fprintf(&b, "\nACCEPTANCE CRITERIA\n%s\n", task.Criteria)
	}

	return strings.TrimSpace(b.String())
}

func buildReviewerSystemPrompt() string {
	return strings.TrimSpace(`You are an automated review gate.
Return a single verdict line:
PASS|<short reason>
or
FAIL|<short reason>

Review principles:
- PASS if task requirements are met.
- FAIL if requirements were not met or output is invalid.
- Prefer concise, actionable feedback.`)
}

func buildReviewerTaskPrompt(planFile string, task StateTask, executorOutput, workdir, planTitle, progress string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "TASK\n")
	fmt.Fprintf(&b, "  Title: %s\n", task.Title)
	fmt.Fprintf(&b, "  ID: %s\n", task.ID)
	if len(task.DependsOn) > 0 {
		fmt.Fprintf(&b, "  Depends on: %s\n", strings.Join(task.DependsOn, ","))
	}

	fmt.Fprintf(&b, "\nPLAN\n")
	fmt.Fprintf(&b, "  File: %s\n", planFile)
	if strings.TrimSpace(planTitle) != "" {
		fmt.Fprintf(&b, "  Title: %s\n", strings.TrimSpace(planTitle))
	}
	if strings.TrimSpace(progress) != "" {
		fmt.Fprintf(&b, "  Progress: %s\n", strings.TrimSpace(progress))
	}
	fmt.Fprintf(&b, "\nWORKDIR\n  %s\n", workdir)

	if task.Description != "" {
		fmt.Fprintf(&b, "\nDESCRIPTION\n%s\n", task.Description)
	}
	if task.Criteria != "" {
		fmt.Fprintf(&b, "\nACCEPTANCE CRITERIA\n%s\n", task.Criteria)
	}

	fmt.Fprintf(&b, "\nEXECUTOR OUTPUT\n%s\n", strings.TrimSpace(executorOutput))

	return strings.TrimSpace(b.String())
}
