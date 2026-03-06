package pipeline

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"

	"github.com/opus-domini/praetor/internal/agent/middleware"
	"github.com/opus-domini/praetor/internal/domain"
)

func actorKey(actor domain.EventActor) string {
	role := strings.TrimSpace(actor.Role)
	if role == "" {
		role = "unknown"
	}
	agent := strings.TrimSpace(actor.Agent)
	if agent == "" {
		agent = "unknown"
	}
	return role + ":" + agent
}

func (run *activeRun) ensureSummaryActor(actor domain.EventActor) domain.ActorStats {
	if run.summary.ByActor == nil {
		run.summary.ByActor = make(map[string]domain.ActorStats)
	}
	return run.summary.ByActor[actorKey(actor)]
}

func (run *activeRun) recordActorCall(actor domain.EventActor, costUSD, durationS float64) {
	if run == nil {
		return
	}
	stats := run.ensureSummaryActor(actor)
	stats.CostUSD += costUSD
	stats.TimeS += durationS
	stats.Calls++
	run.summary.ByActor[actorKey(actor)] = stats
}

func (run *activeRun) recordActorRetry(actor domain.EventActor) {
	if run == nil {
		return
	}
	stats := run.ensureSummaryActor(actor)
	stats.Retries++
	run.summary.ByActor[actorKey(actor)] = stats
}

func (run *activeRun) recordActorStall(actor domain.EventActor) {
	if run == nil {
		return
	}
	stats := run.ensureSummaryActor(actor)
	stats.Stalls++
	run.summary.ByActor[actorKey(actor)] = stats
	run.summary.Stalls++
}

func (run *activeRun) finalizeSummary(eventsPath string) {
	if run == nil {
		return
	}
	run.summary.TotalCostUSD = run.totalCost
	run.summary.TotalTimeS = run.stats.TotalDuration.Seconds()
	run.summary.TasksDone = run.stats.TasksDone
	run.summary.TasksFailed = run.state.FailedCount()
	run.summary.TasksRetried = run.stats.TasksRejected
	run.summary.Fallbacks = countFallbackEvents(eventsPath)
}

func countFallbackEvents(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)
	for scanner.Scan() {
		var event middleware.ExecutionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == middleware.EventAgentFallback {
			count++
		}
	}
	return count
}
