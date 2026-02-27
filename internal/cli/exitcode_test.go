package cli

import (
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
)

func TestExitCodeForOutcome(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		outcome domain.RunOutcome
		want    int
	}{
		{name: "success", outcome: domain.RunSuccess, want: 0},
		{name: "partial", outcome: domain.RunPartial, want: 3},
		{name: "failed", outcome: domain.RunFailed, want: 1},
		{name: "canceled", outcome: domain.RunCanceled, want: 2},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCodeForOutcome(tc.outcome); got != tc.want {
				t.Fatalf("exitCodeForOutcome(%s)=%d, want %d", tc.outcome, got, tc.want)
			}
		})
	}
}
