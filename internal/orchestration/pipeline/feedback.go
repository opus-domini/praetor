package pipeline

import (
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	"github.com/opus-domini/praetor/internal/prompt"
)

func legacyFeedback(taskID string, attempt int, reason string) domain.TaskFeedback {
	return domain.TaskFeedback{
		TaskID:    strings.TrimSpace(taskID),
		Attempt:   attempt,
		Phase:     promptPhaseReview,
		Actor:     domain.EventActor{Role: "system"},
		Verdict:   "fail",
		Reason:    strings.TrimSpace(reason),
		Hints:     []string{strings.TrimSpace(reason)},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

func buildTaskFeedback(taskID string, attempt int, phase string, actor domain.EventActor, verdict, reason string, hints []string, gateOutput string) *domain.TaskFeedback {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil
	}
	normalizedHints := make([]string, 0, len(hints))
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		normalizedHints = append(normalizedHints, hint)
	}
	return &domain.TaskFeedback{
		TaskID:     strings.TrimSpace(taskID),
		Attempt:    attempt,
		Phase:      strings.TrimSpace(phase),
		Actor:      actor,
		Verdict:    strings.TrimSpace(verdict),
		Reason:     reason,
		Hints:      normalizedHints,
		GateOutput: truncateFeedbackOutput(gateOutput),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
}

func truncateFeedbackOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 2000 {
		return output
	}
	return strings.TrimSpace(output[:2000])
}

func trimFeedbackHistoryForPrompt(
	budget int,
	engine *prompt.Engine,
	planFile string,
	taskIndex int,
	task domain.StateTask,
	feedback []domain.TaskFeedback,
	retryCount int,
	planTitle, progress, workdir string,
	requiredGates []string,
	evidenceFormat string,
) ([]domain.TaskFeedback, []string) {
	if budget <= 0 || len(feedback) == 0 {
		return feedback, nil
	}
	history := feedback
	for len(history) > 0 {
		promptText := BuildExecutorTaskPrompt(engine, planFile, taskIndex, task, history, retryCount, planTitle, progress, workdir, requiredGates, evidenceFormat)
		if len(promptText) <= budget {
			if len(history) == len(feedback) {
				return history, nil
			}
			return history, []string{"previous_feedback"}
		}
		history = history[1:]
	}
	return nil, []string{"previous_feedback"}
}

func taskActor(role string, agent domain.Agent, model string) *domain.EventActor {
	return &domain.EventActor{
		Role:  strings.TrimSpace(role),
		Agent: strings.TrimSpace(string(agent)),
		Model: strings.TrimSpace(model),
	}
}

func gateActor(name string) *domain.EventActor {
	return &domain.EventActor{
		Role:  "gate",
		Agent: strings.TrimSpace(name),
	}
}
