package domain

import "testing"

// ---------------------------------------------------------------------------
// NormalizeAgent
// ---------------------------------------------------------------------------

func TestNormalizeAgentLowercase(t *testing.T) {
	t.Parallel()
	got := NormalizeAgent("CLAUDE")
	if got != AgentClaude {
		t.Fatalf("NormalizeAgent(%q) = %q, want %q", "CLAUDE", got, AgentClaude)
	}
}

func TestNormalizeAgentTrimsWhitespace(t *testing.T) {
	t.Parallel()
	got := NormalizeAgent("  codex  ")
	if got != AgentCodex {
		t.Fatalf("NormalizeAgent(%q) = %q, want %q", "  codex  ", got, AgentCodex)
	}
}

func TestNormalizeAgentMixedCase(t *testing.T) {
	t.Parallel()
	got := NormalizeAgent(" Gemini ")
	if got != AgentGemini {
		t.Fatalf("NormalizeAgent(%q) = %q, want %q", " Gemini ", got, AgentGemini)
	}
}

func TestNormalizeAgentAlreadyNormalized(t *testing.T) {
	t.Parallel()
	got := NormalizeAgent("ollama")
	if got != AgentOllama {
		t.Fatalf("NormalizeAgent(%q) = %q, want %q", "ollama", got, AgentOllama)
	}
}

func TestNormalizeAgentEmpty(t *testing.T) {
	t.Parallel()
	got := NormalizeAgent("")
	if got != "" {
		t.Fatalf("NormalizeAgent(%q) = %q, want %q", "", got, Agent(""))
	}
}

func TestNormalizeAgentOnlySpaces(t *testing.T) {
	t.Parallel()
	got := NormalizeAgent("   ")
	if got != "" {
		t.Fatalf("NormalizeAgent(%q) = %q, want %q", "   ", got, Agent(""))
	}
}

// ---------------------------------------------------------------------------
// State.DoneCount / FailedCount / ActiveCount
// ---------------------------------------------------------------------------

func TestStateDoneCount(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskDone},
		{ID: "c", Status: TaskPending},
		{ID: "d", Status: TaskFailed},
	}}
	if got := s.DoneCount(); got != 2 {
		t.Fatalf("DoneCount() = %d, want 2", got)
	}
}

func TestStateDoneCountEmpty(t *testing.T) {
	t.Parallel()
	s := State{}
	if got := s.DoneCount(); got != 0 {
		t.Fatalf("DoneCount() = %d, want 0", got)
	}
}

func TestStateFailedCount(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskFailed},
		{ID: "b", Status: TaskDone},
		{ID: "c", Status: TaskFailed},
	}}
	if got := s.FailedCount(); got != 2 {
		t.Fatalf("FailedCount() = %d, want 2", got)
	}
}

func TestStateFailedCountNone(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskPending},
	}}
	if got := s.FailedCount(); got != 0 {
		t.Fatalf("FailedCount() = %d, want 0", got)
	}
}

func TestStateActiveCount(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskPending},
		{ID: "c", Status: TaskExecuting},
		{ID: "d", Status: TaskFailed},
		{ID: "e", Status: TaskReviewing},
	}}
	// 5 total - 1 done - 1 failed = 3 active
	if got := s.ActiveCount(); got != 3 {
		t.Fatalf("ActiveCount() = %d, want 3", got)
	}
}

func TestStateActiveCountAllTerminal(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskFailed},
	}}
	if got := s.ActiveCount(); got != 0 {
		t.Fatalf("ActiveCount() = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Transition
// ---------------------------------------------------------------------------

func TestTransitionPendingToExecuting(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskPending, TaskExecuting); err != nil {
		t.Fatalf("Transition(pending, executing) unexpected error: %v", err)
	}
}

func TestTransitionPendingToFailed(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskPending, TaskFailed); err != nil {
		t.Fatalf("Transition(pending, failed) unexpected error: %v", err)
	}
}

func TestTransitionExecutingToReviewing(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskExecuting, TaskReviewing); err != nil {
		t.Fatalf("Transition(executing, reviewing) unexpected error: %v", err)
	}
}

func TestTransitionExecutingToDone(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskExecuting, TaskDone); err != nil {
		t.Fatalf("Transition(executing, done) unexpected error: %v", err)
	}
}

func TestTransitionExecutingToPending(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskExecuting, TaskPending); err != nil {
		t.Fatalf("Transition(executing, pending) unexpected error: %v", err)
	}
}

func TestTransitionExecutingToFailed(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskExecuting, TaskFailed); err != nil {
		t.Fatalf("Transition(executing, failed) unexpected error: %v", err)
	}
}

func TestTransitionReviewingToDone(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskReviewing, TaskDone); err != nil {
		t.Fatalf("Transition(reviewing, done) unexpected error: %v", err)
	}
}

func TestTransitionReviewingToPending(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskReviewing, TaskPending); err != nil {
		t.Fatalf("Transition(reviewing, pending) unexpected error: %v", err)
	}
}

func TestTransitionReviewingToFailed(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskReviewing, TaskFailed); err != nil {
		t.Fatalf("Transition(reviewing, failed) unexpected error: %v", err)
	}
}

func TestTransitionInvalidDoneToPending(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskDone, TaskPending); err == nil {
		t.Fatal("Transition(done, pending) should fail but got nil")
	}
}

func TestTransitionInvalidFailedToPending(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskFailed, TaskPending); err == nil {
		t.Fatal("Transition(failed, pending) should fail but got nil")
	}
}

func TestTransitionInvalidPendingToDone(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskPending, TaskDone); err == nil {
		t.Fatal("Transition(pending, done) should fail but got nil")
	}
}

func TestTransitionInvalidPendingToReviewing(t *testing.T) {
	t.Parallel()
	if err := Transition(TaskPending, TaskReviewing); err == nil {
		t.Fatal("Transition(pending, reviewing) should fail but got nil")
	}
}

func TestTransitionUnknownSourceState(t *testing.T) {
	t.Parallel()
	if err := Transition("bogus", TaskPending); err == nil {
		t.Fatal("Transition(bogus, pending) should fail but got nil")
	}
}

// ---------------------------------------------------------------------------
// IsTerminal
// ---------------------------------------------------------------------------

func TestIsTerminalDone(t *testing.T) {
	t.Parallel()
	if !IsTerminal(TaskDone) {
		t.Fatal("IsTerminal(done) = false, want true")
	}
}

func TestIsTerminalFailed(t *testing.T) {
	t.Parallel()
	if !IsTerminal(TaskFailed) {
		t.Fatal("IsTerminal(failed) = false, want true")
	}
}

func TestIsTerminalPending(t *testing.T) {
	t.Parallel()
	if IsTerminal(TaskPending) {
		t.Fatal("IsTerminal(pending) = true, want false")
	}
}

func TestIsTerminalExecuting(t *testing.T) {
	t.Parallel()
	if IsTerminal(TaskExecuting) {
		t.Fatal("IsTerminal(executing) = true, want false")
	}
}

func TestIsTerminalReviewing(t *testing.T) {
	t.Parallel()
	if IsTerminal(TaskReviewing) {
		t.Fatal("IsTerminal(reviewing) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// NormalizeStatus
// ---------------------------------------------------------------------------

func TestNormalizeStatusPending(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus(TaskPending); got != TaskPending {
		t.Fatalf("NormalizeStatus(pending) = %q, want %q", got, TaskPending)
	}
}

func TestNormalizeStatusDone(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus(TaskDone); got != TaskDone {
		t.Fatalf("NormalizeStatus(done) = %q, want %q", got, TaskDone)
	}
}

func TestNormalizeStatusFailed(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus(TaskFailed); got != TaskFailed {
		t.Fatalf("NormalizeStatus(failed) = %q, want %q", got, TaskFailed)
	}
}

func TestNormalizeStatusExecutingResetsToPending(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus(TaskExecuting); got != TaskPending {
		t.Fatalf("NormalizeStatus(executing) = %q, want %q", got, TaskPending)
	}
}

func TestNormalizeStatusReviewingResetsToPending(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus(TaskReviewing); got != TaskPending {
		t.Fatalf("NormalizeStatus(reviewing) = %q, want %q", got, TaskPending)
	}
}

func TestNormalizeStatusUnknownDefaultsToPending(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus("garbage"); got != TaskPending {
		t.Fatalf("NormalizeStatus(garbage) = %q, want %q", got, TaskPending)
	}
}

func TestNormalizeStatusEmptyDefaultsToPending(t *testing.T) {
	t.Parallel()
	if got := NormalizeStatus(""); got != TaskPending {
		t.Fatalf("NormalizeStatus(\"\") = %q, want %q", got, TaskPending)
	}
}

// ---------------------------------------------------------------------------
// NextRunnableTask
// ---------------------------------------------------------------------------

func TestNextRunnableTaskNoDeps(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskPending},
		{ID: "b", Status: TaskPending},
	}}
	idx, task, ok := NextRunnableTask(s)
	if !ok {
		t.Fatal("NextRunnableTask returned ok=false, want true")
	}
	if idx != 0 || task.ID != "a" {
		t.Fatalf("NextRunnableTask = (idx=%d, id=%s), want (0, a)", idx, task.ID)
	}
}

func TestNextRunnableTaskSkipsDone(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskPending},
	}}
	idx, task, ok := NextRunnableTask(s)
	if !ok {
		t.Fatal("NextRunnableTask returned ok=false, want true")
	}
	if idx != 1 || task.ID != "b" {
		t.Fatalf("NextRunnableTask = (idx=%d, id=%s), want (1, b)", idx, task.ID)
	}
}

func TestNextRunnableTaskSkipsExecuting(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskExecuting},
		{ID: "b", Status: TaskPending},
	}}
	idx, task, ok := NextRunnableTask(s)
	if !ok {
		t.Fatal("NextRunnableTask returned ok=false, want true")
	}
	if idx != 1 || task.ID != "b" {
		t.Fatalf("NextRunnableTask = (idx=%d, id=%s), want (1, b)", idx, task.ID)
	}
}

func TestNextRunnableTaskBlockedByDependency(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskPending},
		{ID: "b", Status: TaskPending, DependsOn: []string{"a"}},
	}}
	idx, task, ok := NextRunnableTask(s)
	if !ok {
		t.Fatal("NextRunnableTask returned ok=false, want true")
	}
	// "a" is runnable (no deps), "b" is blocked
	if idx != 0 || task.ID != "a" {
		t.Fatalf("NextRunnableTask = (idx=%d, id=%s), want (0, a)", idx, task.ID)
	}
}

func TestNextRunnableTaskDependencySatisfied(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskPending, DependsOn: []string{"a"}},
	}}
	idx, task, ok := NextRunnableTask(s)
	if !ok {
		t.Fatal("NextRunnableTask returned ok=false, want true")
	}
	if idx != 1 || task.ID != "b" {
		t.Fatalf("NextRunnableTask = (idx=%d, id=%s), want (1, b)", idx, task.ID)
	}
}

func TestNextRunnableTaskAllDone(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskDone},
	}}
	_, _, ok := NextRunnableTask(s)
	if ok {
		t.Fatal("NextRunnableTask returned ok=true for all-done state, want false")
	}
}

func TestNextRunnableTaskAllBlocked(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskPending, DependsOn: []string{"b"}},
		{ID: "b", Status: TaskPending, DependsOn: []string{"a"}},
	}}
	_, _, ok := NextRunnableTask(s)
	if ok {
		t.Fatal("NextRunnableTask returned ok=true for circular deps, want false")
	}
}

func TestNextRunnableTaskEmptyState(t *testing.T) {
	t.Parallel()
	s := State{}
	_, _, ok := NextRunnableTask(s)
	if ok {
		t.Fatal("NextRunnableTask returned ok=true for empty state, want false")
	}
}

// ---------------------------------------------------------------------------
// RunnableTasks
// ---------------------------------------------------------------------------

func TestRunnableTasksMultiple(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskPending},
		{ID: "b", Status: TaskDone},
		{ID: "c", Status: TaskPending},
		{ID: "d", Status: TaskPending, DependsOn: []string{"b", "c"}},
	}}
	got := RunnableTasks(s)
	// a is runnable (no deps), c is runnable (no deps), d is blocked (c not done)
	if len(got) != 2 {
		t.Fatalf("RunnableTasks() returned %d tasks, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("RunnableTasks() = [%s, %s], want [a, c]", got[0].ID, got[1].ID)
	}
}

func TestRunnableTasksNone(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskDone},
		{ID: "b", Status: TaskFailed},
	}}
	got := RunnableTasks(s)
	if len(got) != 0 {
		t.Fatalf("RunnableTasks() returned %d tasks, want 0", len(got))
	}
}

// ---------------------------------------------------------------------------
// BlockedTasksReport
// ---------------------------------------------------------------------------

func TestBlockedTasksReportBasic(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "a", Status: TaskPending},
		{ID: "b", Status: TaskPending, DependsOn: []string{"a"}, Title: "Task B"},
	}}
	report := BlockedTasksReport(s, 5)
	// "a" is runnable (not blocked), "b" is blocked on "a"
	if len(report) != 1 {
		t.Fatalf("BlockedTasksReport() returned %d entries, want 1", len(report))
	}
}

func TestBlockedTasksReportRespectsLimit(t *testing.T) {
	t.Parallel()
	s := State{Tasks: []StateTask{
		{ID: "x", Status: TaskExecuting},
		{ID: "a", Status: TaskPending, DependsOn: []string{"x"}, Title: "A"},
		{ID: "b", Status: TaskPending, DependsOn: []string{"x"}, Title: "B"},
		{ID: "c", Status: TaskPending, DependsOn: []string{"x"}, Title: "C"},
	}}
	report := BlockedTasksReport(s, 2)
	if len(report) != 2 {
		t.Fatalf("BlockedTasksReport(limit=2) returned %d entries, want 2", len(report))
	}
}

func TestBlockedTasksReportDefaultLimit(t *testing.T) {
	t.Parallel()
	s := State{}
	// limit <= 0 should default to 5 (no crash)
	report := BlockedTasksReport(s, 0)
	if report == nil {
		t.Fatal("BlockedTasksReport(limit=0) returned nil, want empty slice")
	}
}

// ---------------------------------------------------------------------------
// ParseExecutorResult
// ---------------------------------------------------------------------------

func TestParseExecutorResultPass(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("some output\nRESULT: PASS\n")
	if got != ExecutorResultPass {
		t.Fatalf("ParseExecutorResult(PASS) = %q, want %q", got, ExecutorResultPass)
	}
}

func TestParseExecutorResultFail(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("some output\nRESULT: FAIL\n")
	if got != ExecutorResultFail {
		t.Fatalf("ParseExecutorResult(FAIL) = %q, want %q", got, ExecutorResultFail)
	}
}

func TestParseExecutorResultUnknownValue(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("RESULT: MAYBE\n")
	if got != ExecutorResultUnknown {
		t.Fatalf("ParseExecutorResult(MAYBE) = %q, want %q", got, ExecutorResultUnknown)
	}
}

func TestParseExecutorResultMissingLine(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("just some output with no result line")
	if got != ExecutorResultUnknown {
		t.Fatalf("ParseExecutorResult(missing) = %q, want %q", got, ExecutorResultUnknown)
	}
}

func TestParseExecutorResultEmpty(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("")
	if got != ExecutorResultUnknown {
		t.Fatalf("ParseExecutorResult(\"\") = %q, want %q", got, ExecutorResultUnknown)
	}
}

func TestParseExecutorResultCaseInsensitive(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("result: pass")
	if got != ExecutorResultPass {
		t.Fatalf("ParseExecutorResult(lowercase) = %q, want %q", got, ExecutorResultPass)
	}
}

func TestParseExecutorResultWithCarriageReturn(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("RESULT: PASS\r\n")
	if got != ExecutorResultPass {
		t.Fatalf("ParseExecutorResult(with CR) = %q, want %q", got, ExecutorResultPass)
	}
}

func TestParseExecutorResultWithLeadingWhitespace(t *testing.T) {
	t.Parallel()
	got := ParseExecutorResult("  RESULT: FAIL  ")
	if got != ExecutorResultFail {
		t.Fatalf("ParseExecutorResult(whitespace) = %q, want %q", got, ExecutorResultFail)
	}
}

// ---------------------------------------------------------------------------
// ParseReviewDecision
// ---------------------------------------------------------------------------

func TestParseReviewDecisionPassWithReason(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("PASS|looks good")
	if !d.Pass {
		t.Fatal("ParseReviewDecision(PASS|...) Pass = false, want true")
	}
	if d.Reason != "looks good" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "looks good")
	}
}

func TestParseReviewDecisionFailWithReason(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("FAIL|needs work")
	if d.Pass {
		t.Fatal("ParseReviewDecision(FAIL|...) Pass = true, want false")
	}
	if d.Reason != "needs work" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "needs work")
	}
}

func TestParseReviewDecisionPassNoReason(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("PASS")
	if !d.Pass {
		t.Fatal("ParseReviewDecision(PASS) Pass = false, want true")
	}
	if d.Reason != "review passed" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "review passed")
	}
}

func TestParseReviewDecisionFailNoReason(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("FAIL")
	if d.Pass {
		t.Fatal("ParseReviewDecision(FAIL) Pass = true, want false")
	}
	if d.Reason != "review failed" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "review failed")
	}
}

func TestParseReviewDecisionEmpty(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("")
	if d.Pass {
		t.Fatal("ParseReviewDecision(\"\") Pass = true, want false")
	}
	if d.Reason != "reviewer output was empty" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "reviewer output was empty")
	}
}

func TestParseReviewDecisionInvalid(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("MAYBE|something")
	if d.Pass {
		t.Fatal("ParseReviewDecision(MAYBE) Pass = true, want false")
	}
	expected := "reviewer output must use PASS|... or FAIL|..."
	if d.Reason != expected {
		t.Fatalf("Reason = %q, want %q", d.Reason, expected)
	}
}

func TestParseReviewDecisionLowercasePass(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("pass|ok")
	if !d.Pass {
		t.Fatal("ParseReviewDecision(pass|ok) Pass = false, want true")
	}
	if d.Reason != "ok" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "ok")
	}
}

func TestParseReviewDecisionMultiline(t *testing.T) {
	t.Parallel()
	// Parser takes the first non-empty line
	d := ParseReviewDecision("\n\nFAIL|missing tests\nextra noise")
	if d.Pass {
		t.Fatal("ParseReviewDecision(multiline FAIL) Pass = true, want false")
	}
	if d.Reason != "missing tests" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "missing tests")
	}
}

func TestParseReviewDecisionPipeInReason(t *testing.T) {
	t.Parallel()
	d := ParseReviewDecision("PASS|looks good | formatting ok")
	if !d.Pass {
		t.Fatal("Pass = false, want true")
	}
	if d.Reason != "looks good | formatting ok" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "looks good | formatting ok")
	}
}

func TestIsReviewerDecisionParseFailure(t *testing.T) {
	t.Parallel()

	parseErr := ParseReviewDecision("MAYBE|something")
	if !IsReviewerDecisionParseFailure(parseErr) {
		t.Fatal("expected parse failure for invalid reviewer contract")
	}

	semanticFail := ParseReviewDecision("FAIL|needs tests")
	if IsReviewerDecisionParseFailure(semanticFail) {
		t.Fatal("did not expect parse failure for valid FAIL decision")
	}
}
