package loop

import "github.com/opus-domini/praetor/internal/orchestration/pipeline"

func buildExecutorSystemPrompt(projectContext string) string {
	return pipeline.BuildExecutorSystemPrompt(projectContext)
}

func buildExecutorTaskPrompt(planFile string, taskIndex int, task StateTask, previousFeedback string, retryCount int, planTitle, progress, workdir string) string {
	return pipeline.BuildExecutorTaskPrompt(planFile, taskIndex, task, previousFeedback, retryCount, planTitle, progress, workdir)
}

func buildReviewerSystemPrompt(projectContext string) string {
	return pipeline.BuildReviewerSystemPrompt(projectContext)
}

func buildReviewerTaskPrompt(planFile string, task StateTask, executorOutput, workdir, planTitle, progress, gitDiff string) string {
	return pipeline.BuildReviewerTaskPrompt(planFile, task, executorOutput, workdir, planTitle, progress, gitDiff)
}

// truncateOutput delegates to pipeline.TruncateOutput.
func truncateOutput(output string, maxLines int) string {
	return pipeline.TruncateOutput(output, maxLines)
}
