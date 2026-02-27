package pipeline

import (
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/prompt"
)

// BuildExecutorSystemPrompt constructs the system prompt for the executor agent.
func BuildExecutorSystemPrompt(engine *prompt.Engine, projectContext string) string {
	if engine != nil {
		if s, err := engine.Render("executor.system", prompt.ExecutorSystemData{
			ProjectContext: projectContext,
		}); err == nil {
			return s
		}
	}
	var b strings.Builder
	if projectContext != "" {
		fmt.Fprintf(&b, "## Project Context\n%s\n\n", projectContext)
	}
	b.WriteString(`## Your Role
You are an autonomous executor agent in an automated task pipeline.
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
	return strings.TrimSpace(b.String())
}

// BuildExecutorTaskPrompt constructs the task prompt for the executor agent.
func BuildExecutorTaskPrompt(engine *prompt.Engine, planFile string, taskIndex int, task domain.StateTask, previousFeedback string, retryCount int, planTitle, progress, workdir string, requiredGates []string, evidenceFormat string) string {
	acceptance := formatAcceptance(task.Acceptance)
	if engine != nil {
		dependsOn := ""
		if len(task.DependsOn) > 0 {
			dependsOn = strings.Join(task.DependsOn, ",")
		}
		if s, err := engine.Render("executor.task", prompt.ExecutorTaskData{
			IsRetry:          previousFeedback != "" && retryCount > 0,
			RetryAttempt:     retryCount,
			PreviousFeedback: previousFeedback,
			TaskTitle:        task.Title,
			TaskID:           task.ID,
			TaskIndex:        taskIndex,
			TaskDependsOn:    dependsOn,
			TaskDescription:  task.Description,
			TaskAcceptance:   acceptance,
			PlanFile:         planFile,
			PlanName:         strings.TrimSpace(planTitle),
			PlanProgress:     strings.TrimSpace(progress),
			Workdir:          workdir,
			GatesRequired:    requiredGates,
			EvidenceFormat:   strings.TrimSpace(evidenceFormat),
		}); err == nil {
			return s
		}
	}
	var b strings.Builder

	if previousFeedback != "" && retryCount > 0 {
		fmt.Fprintf(&b, "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n")
		fmt.Fprintf(&b, "!! RETRY — attempt %d\n", retryCount+1)
		fmt.Fprintf(&b, "!! Your previous attempt was REJECTED.\n")
		fmt.Fprintf(&b, "!! Read the feedback below carefully and fix ALL issues.\n")
		fmt.Fprintf(&b, "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n\n")
		fmt.Fprintf(&b, "PREVIOUS FEEDBACK:\n%s\n\n", previousFeedback)
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
	if acceptance != "" {
		fmt.Fprintf(&b, "\nACCEPTANCE CRITERIA\n%s\n", acceptance)
	}
	if len(requiredGates) > 0 {
		if strings.TrimSpace(evidenceFormat) == "" {
			evidenceFormat = "gates_v1"
		}
		fmt.Fprintf(&b, "\nQUALITY GATES (required)\n")
		for _, gate := range requiredGates {
			gate = strings.TrimSpace(gate)
			if gate == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", gate)
		}
		fmt.Fprintf(&b, "\nEmit evidence using format %q:\n", strings.TrimSpace(evidenceFormat))
		fmt.Fprintf(&b, "GATES:\n")
		fmt.Fprintf(&b, "- tests: PASS (details)\n")
		fmt.Fprintf(&b, "- lint: PASS (details)\n")
	}

	return strings.TrimSpace(b.String())
}

// BuildReviewerSystemPrompt constructs the system prompt for the reviewer agent.
func BuildReviewerSystemPrompt(engine *prompt.Engine, projectContext string) string {
	if engine != nil {
		if s, err := engine.Render("reviewer.system", prompt.ReviewerSystemData{
			ProjectContext: projectContext,
		}); err == nil {
			return s
		}
	}
	var b strings.Builder
	if projectContext != "" {
		fmt.Fprintf(&b, "## Project Context\n%s\n\n", projectContext)
	}
	b.WriteString(`## Your Role
You are an automated review gate.
Return a single verdict line:
PASS|<short reason>
or
FAIL|<short reason>

Review principles:
- PASS if task requirements are met.
- FAIL if requirements were not met or output is invalid.
- Prefer concise, actionable feedback.`)
	return strings.TrimSpace(b.String())
}

// BuildReviewerTaskPrompt constructs the task prompt for the reviewer agent.
func BuildReviewerTaskPrompt(engine *prompt.Engine, planFile string, task domain.StateTask, executorOutput, workdir, planTitle, progress, gitDiff string) string {
	acceptance := formatAcceptance(task.Acceptance)
	if engine != nil {
		dependsOn := ""
		if len(task.DependsOn) > 0 {
			dependsOn = strings.Join(task.DependsOn, ",")
		}
		if s, err := engine.Render("reviewer.task", prompt.ReviewerTaskData{
			TaskTitle:       task.Title,
			TaskID:          task.ID,
			TaskDependsOn:   dependsOn,
			TaskDescription: task.Description,
			TaskAcceptance:  acceptance,
			PlanFile:        planFile,
			PlanName:        strings.TrimSpace(planTitle),
			PlanProgress:    strings.TrimSpace(progress),
			Workdir:         workdir,
			ExecutorOutput:  strings.TrimSpace(executorOutput),
			GitDiff:         strings.TrimSpace(gitDiff),
		}); err == nil {
			return s
		}
	}
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
	if acceptance != "" {
		fmt.Fprintf(&b, "\nACCEPTANCE CRITERIA\n%s\n", acceptance)
	}

	fmt.Fprintf(&b, "\nEXECUTOR OUTPUT\n%s\n", strings.TrimSpace(executorOutput))

	if strings.TrimSpace(gitDiff) != "" {
		fmt.Fprintf(&b, "\nGIT DIFF\n%s\n", strings.TrimSpace(gitDiff))
	}

	return strings.TrimSpace(b.String())
}

// TruncateOutput keeps only the last maxLines lines of output.
func TruncateOutput(output string, maxLines int) string {
	output = strings.TrimSpace(output)
	if output == "" || maxLines <= 0 {
		return output
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func formatAcceptance(items []string) string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		normalized = append(normalized, "- "+item)
	}
	return strings.Join(normalized, "\n")
}
