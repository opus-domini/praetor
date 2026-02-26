package loop

import "github.com/opus-domini/praetor/internal/domain"

// NextRunnableTask delegates to domain.NextRunnableTask.
func NextRunnableTask(state State) (int, StateTask, bool) {
	return domain.NextRunnableTask(state)
}

// RunnableTasks delegates to domain.RunnableTasks.
func RunnableTasks(state State) []StateTask {
	return domain.RunnableTasks(state)
}

// BlockedTasksReport delegates to domain.BlockedTasksReport.
func BlockedTasksReport(state State, limit int) []string {
	return domain.BlockedTasksReport(state, limit)
}
