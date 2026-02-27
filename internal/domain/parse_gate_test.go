package domain

import "testing"

func TestParseGateEvidence(t *testing.T) {
	t.Parallel()
	output := `RESULT: PASS
GATES:
- tests: PASS (42 tests)
- lint: FAIL (2 issues)
`
	gates := ParseGateEvidence(output)
	if len(gates) != 2 {
		t.Fatalf("expected 2 gates, got %d", len(gates))
	}
	if gates["tests"].Status != "PASS" {
		t.Fatalf("expected tests PASS, got %+v", gates["tests"])
	}
	if gates["lint"].Status != "FAIL" {
		t.Fatalf("expected lint FAIL, got %+v", gates["lint"])
	}
}

func TestParseGateEvidenceMissingBlock(t *testing.T) {
	t.Parallel()
	gates := ParseGateEvidence("RESULT: PASS")
	if len(gates) != 0 {
		t.Fatalf("expected empty map, got %d", len(gates))
	}
}
