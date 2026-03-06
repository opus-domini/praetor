package domain

import "fmt"

// NextRunnableTask returns the first open task whose dependencies are done.
func NextRunnableTask(state State) (int, StateTask, bool) {
	done := doneSet(state)
	for idx, task := range state.Tasks {
		if task.Status != TaskPending {
			continue
		}
		if dependenciesDone(task, done) {
			return idx, task, true
		}
	}
	return -1, StateTask{}, false
}

// RunnableTasks returns all currently runnable open tasks.
func RunnableTasks(state State) []StateTask {
	indices := RunnableTaskIndices(state, 0)
	tasks := make([]StateTask, 0, len(indices))
	for _, index := range indices {
		tasks = append(tasks, state.Tasks[index])
	}
	return tasks
}

// RunnableTaskIndices returns deterministic indices for the current runnable wave.
// A limit <= 0 returns the full frontier.
func RunnableTaskIndices(state State, limit int) []int {
	done := doneSet(state)
	indices := make([]int, 0)
	for index, task := range state.Tasks {
		if task.Status != TaskPending {
			continue
		}
		if dependenciesDone(task, done) {
			indices = append(indices, index)
			if limit > 0 && len(indices) >= limit {
				break
			}
		}
	}
	return indices
}

// BlockedTasksReport returns a compact report for blocked open tasks.
func BlockedTasksReport(state State, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	done := doneSet(state)
	report := make([]string, 0, limit)

	for idx, task := range state.Tasks {
		if len(report) >= limit {
			break
		}
		if task.Status != TaskPending {
			continue
		}

		missing := make([]string, 0)
		for _, dep := range task.DependsOn {
			if _, ok := done[dep]; !ok {
				missing = append(missing, dep)
			}
		}
		if len(missing) == 0 {
			continue
		}
		report = append(report, fmt.Sprintf("index=%d id=%s depends_missing=%v task=%s", idx, task.ID, missing, task.Title))
	}
	return report
}

func doneSet(state State) map[string]struct{} {
	done := make(map[string]struct{}, len(state.Tasks))
	for _, task := range state.Tasks {
		if task.Status != TaskDone {
			continue
		}
		done[task.ID] = struct{}{}
	}
	return done
}

func dependenciesDone(task StateTask, done map[string]struct{}) bool {
	for _, dep := range task.DependsOn {
		if _, ok := done[dep]; !ok {
			return false
		}
	}
	return true
}
