package domain

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Reviewer — JSON structured output
// ---------------------------------------------------------------------------

func TestParseReviewDecisionStructuredPass(t *testing.T) {
	t.Parallel()
	output := `{"decision":"PASS","reason":"all criteria met"}`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected Pass = true")
	}
	if d.Reason != "all criteria met" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "all criteria met")
	}
}

func TestParseReviewDecisionStructuredFail(t *testing.T) {
	t.Parallel()
	output := `{"decision":"FAIL","reason":"tests broken","hints":["fix test_foo","add coverage"]}`
	d := ParseReviewDecision(output)
	if d.Pass {
		t.Fatal("expected Pass = false")
	}
	if d.Reason != "tests broken" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "tests broken")
	}
	if len(d.Hints) != 2 {
		t.Fatalf("Hints len = %d, want 2", len(d.Hints))
	}
	if d.Hints[0] != "fix test_foo" || d.Hints[1] != "add coverage" {
		t.Fatalf("unexpected hints: %+v", d.Hints)
	}
}

func TestParseReviewDecisionStructuredFailNoHints(t *testing.T) {
	t.Parallel()
	output := `{"decision":"FAIL","reason":"missing implementation"}`
	d := ParseReviewDecision(output)
	if d.Pass {
		t.Fatal("expected Pass = false")
	}
	if d.Reason != "missing implementation" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "missing implementation")
	}
	// When no hints provided, reason is used as the sole hint
	if len(d.Hints) != 1 || d.Hints[0] != "missing implementation" {
		t.Fatalf("expected reason as default hint, got: %+v", d.Hints)
	}
}

func TestParseReviewDecisionStructuredEmptyReason(t *testing.T) {
	t.Parallel()
	output := `{"decision":"PASS","reason":""}`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected Pass = true")
	}
	if d.Reason != "review passed" {
		t.Fatalf("Reason = %q, want default %q", d.Reason, "review passed")
	}
}

func TestParseReviewDecisionStructuredPrecedesText(t *testing.T) {
	t.Parallel()
	// Structured output line should be picked over text verdict
	output := `{"decision":"PASS","reason":"structured wins"}
Analysis text here
FAIL|this should be ignored`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected structured PASS to take precedence")
	}
	if d.Reason != "structured wins" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "structured wins")
	}
}

func TestParseReviewDecisionStructuredWithAnalysisPrefix(t *testing.T) {
	t.Parallel()
	// Real-world: structured output prepended by adapter, followed by text analysis
	output := `{"decision":"PASS","reason":"all good"}
All acceptance criteria verified:
1. Function exists
2. Tests pass`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected Pass = true")
	}
	if d.Reason != "all good" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "all good")
	}
}

func TestParseReviewDecisionStructuredCaseInsensitive(t *testing.T) {
	t.Parallel()
	output := `{"decision":"pass","reason":"ok"}`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected Pass = true for lowercase 'pass'")
	}
}

func TestParseReviewDecisionStructuredEmptyHints(t *testing.T) {
	t.Parallel()
	output := `{"decision":"FAIL","reason":"broken","hints":[]}`
	d := ParseReviewDecision(output)
	if d.Pass {
		t.Fatal("expected Pass = false")
	}
	// Empty hints array → reason as default hint
	if len(d.Hints) != 1 || d.Hints[0] != "broken" {
		t.Fatalf("expected reason as default hint, got: %+v", d.Hints)
	}
}

func TestParseReviewDecisionStructuredWhitespaceHints(t *testing.T) {
	t.Parallel()
	output := `{"decision":"FAIL","reason":"broken","hints":["  ","valid hint",""]}`
	d := ParseReviewDecision(output)
	if len(d.Hints) != 1 || d.Hints[0] != "valid hint" {
		t.Fatalf("expected whitespace-only hints to be filtered, got: %+v", d.Hints)
	}
}

func TestParseReviewDecisionStructuredInvalidJSON(t *testing.T) {
	t.Parallel()
	// Invalid JSON should fall through to text parsing
	output := `{invalid json}
PASS|text fallback works`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected text fallback to PASS")
	}
	if d.Reason != "text fallback works" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "text fallback works")
	}
}

func TestParseReviewDecisionStructuredWrongSchema(t *testing.T) {
	t.Parallel()
	// Valid JSON but wrong schema (no decision field) should fall through
	output := `{"foo":"bar"}
PASS|text fallback`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected text fallback to PASS")
	}
}

func TestParseReviewDecisionStructuredInvalidDecision(t *testing.T) {
	t.Parallel()
	// Valid JSON with invalid decision value should fall through
	output := `{"decision":"MAYBE","reason":"unsure"}
FAIL|text fallback`
	d := ParseReviewDecision(output)
	if d.Pass {
		t.Fatal("expected text fallback to FAIL")
	}
	if d.Reason != "text fallback" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "text fallback")
	}
}

// ---------------------------------------------------------------------------
// Executor — JSON structured output
// ---------------------------------------------------------------------------

func TestParseExecutorResultStructuredPass(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"all done"}`
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatalf("got %q, want %q", got, ExecutorResultPass)
	}
}

func TestParseExecutorResultStructuredFail(t *testing.T) {
	t.Parallel()
	output := `{"result":"FAIL","summary":"compilation error"}`
	got := ParseExecutorResult(output)
	if got != ExecutorResultFail {
		t.Fatalf("got %q, want %q", got, ExecutorResultFail)
	}
}

func TestParseExecutorResultStructuredCaseInsensitive(t *testing.T) {
	t.Parallel()
	output := `{"result":"pass","summary":"ok"}`
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatalf("got %q, want %q", got, ExecutorResultPass)
	}
}

func TestParseExecutorResultStructuredPrecedesText(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"done"}
Some output text
RESULT: FAIL`
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatal("expected structured PASS to take precedence over text FAIL")
	}
}

func TestParseExecutorResultStructuredWithAnalysis(t *testing.T) {
	t.Parallel()
	output := `{"result":"FAIL","summary":"tests broken"}
I tried to fix the issue but the tests are still failing.
The main problem is...`
	got := ParseExecutorResult(output)
	if got != ExecutorResultFail {
		t.Fatalf("got %q, want %q", got, ExecutorResultFail)
	}
}

func TestParseExecutorResultStructuredInvalidJSON(t *testing.T) {
	t.Parallel()
	// Invalid JSON falls through to text
	output := `{broken json
RESULT: PASS`
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatalf("got %q, want %q after text fallback", got, ExecutorResultPass)
	}
}

func TestParseExecutorResultStructuredWrongSchema(t *testing.T) {
	t.Parallel()
	// Valid JSON but no result field
	output := `{"foo":"bar"}
RESULT: FAIL`
	got := ParseExecutorResult(output)
	if got != ExecutorResultFail {
		t.Fatalf("got %q, want %q after text fallback", got, ExecutorResultFail)
	}
}

func TestParseExecutorResultStructuredInvalidResult(t *testing.T) {
	t.Parallel()
	// Valid JSON with unrecognized result
	output := `{"result":"MAYBE","summary":"unsure"}
RESULT: PASS`
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatalf("got %q, want %q after text fallback", got, ExecutorResultPass)
	}
}

// ---------------------------------------------------------------------------
// Gate Evidence — JSON structured output
// ---------------------------------------------------------------------------

func TestParseGateEvidenceStructured(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"all good","gates":{"tests":"PASS","lint":"PASS","standards":"FAIL"}}`
	gates := ParseGateEvidence(output)
	if len(gates) != 3 {
		t.Fatalf("expected 3 gates, got %d: %+v", len(gates), gates)
	}
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS", gates["tests"].Status)
	}
	if gates["lint"].Status != "PASS" {
		t.Fatalf("lint = %q, want PASS", gates["lint"].Status)
	}
	if gates["standards"].Status != "FAIL" {
		t.Fatalf("standards = %q, want FAIL", gates["standards"].Status)
	}
}

func TestParseGateEvidenceStructuredPartial(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"ok","gates":{"tests":"PASS"}}`
	gates := ParseGateEvidence(output)
	if len(gates) != 1 {
		t.Fatalf("expected 1 gate, got %d: %+v", len(gates), gates)
	}
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS", gates["tests"].Status)
	}
}

func TestParseGateEvidenceStructuredNoGates(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"ok"}`
	gates := ParseGateEvidence(output)
	if len(gates) != 0 {
		t.Fatalf("expected 0 gates, got %d", len(gates))
	}
}

func TestParseGateEvidenceStructuredPrecedesText(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"ok","gates":{"tests":"PASS","lint":"FAIL"}}
GATES:
- tests: FAIL
- lint: PASS`
	gates := ParseGateEvidence(output)
	// Structured output should win
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS from structured", gates["tests"].Status)
	}
	if gates["lint"].Status != "FAIL" {
		t.Fatalf("lint = %q, want FAIL from structured", gates["lint"].Status)
	}
}

func TestParseGateEvidenceStructuredFallbackToText(t *testing.T) {
	t.Parallel()
	output := `some text output
GATES:
- tests: PASS
- lint: FAIL`
	gates := ParseGateEvidence(output)
	if len(gates) != 2 {
		t.Fatalf("expected 2 gates from text fallback, got %d", len(gates))
	}
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS", gates["tests"].Status)
	}
	if gates["lint"].Status != "FAIL" {
		t.Fatalf("lint = %q, want FAIL", gates["lint"].Status)
	}
}

func TestParseGateEvidenceStructuredCaseInsensitive(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"ok","gates":{"tests":"pass","lint":"fail"}}`
	gates := ParseGateEvidence(output)
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS", gates["tests"].Status)
	}
	if gates["lint"].Status != "FAIL" {
		t.Fatalf("lint = %q, want FAIL", gates["lint"].Status)
	}
}

// ---------------------------------------------------------------------------
// Integration: structured output prepended to text (as adapter does)
// ---------------------------------------------------------------------------

func TestParseReviewDecisionAdapterPrepend(t *testing.T) {
	t.Parallel()
	// Simulates what ClaudeCLI.run() produces: structured JSON on first line,
	// then the text analysis from the agent
	output := `{"decision":"PASS","reason":"criteria met","hints":[]}
All acceptance criteria verified:

1. **Function consolidated** — present in _bootstrap.sh
2. **Duplicates removed** — gone from files.sh, system.sh
3. **Tests pass** — 262 pass, 5 fail (pre-existing)

PASS|All acceptance criteria met: single function, duplicates removed, tests unchanged.`
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected Pass = true from structured output")
	}
	if d.Reason != "criteria met" {
		t.Fatalf("Reason = %q, want %q (from structured, not text)", d.Reason, "criteria met")
	}
}

func TestParseExecutorResultAdapterPrepend(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"implemented and tested"}
I've implemented the requested changes. Here's what I did:
- Added the new function
- Updated tests
- All tests pass

RESULT: PASS
SUMMARY: Changes implemented
TESTS: make test — all pass`
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatalf("got %q, want %q from structured output", got, ExecutorResultPass)
	}
}

func TestParseGateEvidenceAdapterPrepend(t *testing.T) {
	t.Parallel()
	output := `{"result":"PASS","summary":"done","gates":{"tests":"PASS","lint":"PASS"}}
Implementation complete.

RESULT: PASS
SUMMARY: All done
TESTS: make test passed

GATES:
- tests: FAIL
- lint: FAIL`
	gates := ParseGateEvidence(output)
	// Structured should win over text
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS from structured", gates["tests"].Status)
	}
	if gates["lint"].Status != "PASS" {
		t.Fatalf("lint = %q, want PASS from structured", gates["lint"].Status)
	}
}

// ---------------------------------------------------------------------------
// Edge cases: ensure text-only parsing still works perfectly
// ---------------------------------------------------------------------------

func TestParseReviewDecisionTextOnlyStillWorks(t *testing.T) {
	t.Parallel()
	// No JSON anywhere — pure text mode (non-Claude agents)
	output := "PASS|all good"
	d := ParseReviewDecision(output)
	if !d.Pass {
		t.Fatal("expected Pass = true in text-only mode")
	}
	if d.Reason != "all good" {
		t.Fatalf("Reason = %q, want %q", d.Reason, "all good")
	}
}

func TestParseExecutorResultTextOnlyStillWorks(t *testing.T) {
	t.Parallel()
	output := "some output\nRESULT: PASS\nSUMMARY: done"
	got := ParseExecutorResult(output)
	if got != ExecutorResultPass {
		t.Fatalf("got %q, want %q in text-only mode", got, ExecutorResultPass)
	}
}

func TestParseGateEvidenceTextOnlyStillWorks(t *testing.T) {
	t.Parallel()
	output := "GATES:\n- tests: PASS\n- lint: FAIL some detail"
	gates := ParseGateEvidence(output)
	if len(gates) != 2 {
		t.Fatalf("expected 2 gates in text-only mode, got %d", len(gates))
	}
	if gates["tests"].Status != "PASS" {
		t.Fatalf("tests = %q, want PASS", gates["tests"].Status)
	}
	if gates["lint"].Status != "FAIL" {
		t.Fatalf("lint = %q, want FAIL", gates["lint"].Status)
	}
}
