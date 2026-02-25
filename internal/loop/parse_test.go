package loop

import "testing"

func TestParseExecutorResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   ExecutorResult
	}{
		{name: "pass", output: "hello\nRESULT: PASS\nSUMMARY: ok", want: ExecutorResultPass},
		{name: "fail", output: "RESULT: FAIL", want: ExecutorResultFail},
		{name: "unknown", output: "no result", want: ExecutorResultUnknown},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseExecutorResult(tc.output); got != tc.want {
				t.Fatalf("unexpected result: got=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestParseReviewDecision(t *testing.T) {
	t.Parallel()

	pass := ParseReviewDecision("PASS|looks good")
	if !pass.Pass || pass.Reason != "looks good" {
		t.Fatalf("unexpected pass decision: %+v", pass)
	}

	fail := ParseReviewDecision("FAIL|missing tests")
	if fail.Pass || fail.Reason != "missing tests" {
		t.Fatalf("unexpected fail decision: %+v", fail)
	}

	invalid := ParseReviewDecision("maybe")
	if invalid.Pass {
		t.Fatalf("expected invalid output to fail")
	}
}
