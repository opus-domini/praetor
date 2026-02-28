package pipeline

import "testing"

func TestStallDetectorIdenticalOutputs(t *testing.T) {
	t.Parallel()
	d := NewStallDetector(3, 0.67)
	if stalled, _ := d.Observe("TASK-1", "execute", "same output"); stalled {
		t.Fatal("did not expect stall on first sample")
	}
	if stalled, _ := d.Observe("TASK-1", "execute", "same output"); stalled {
		t.Fatal("did not expect stall on second sample")
	}
	stalled, similarity := d.Observe("TASK-1", "execute", "same output")
	if !stalled {
		t.Fatal("expected stall on third identical sample")
	}
	if similarity < 0.99 {
		t.Fatalf("expected high similarity, got %.2f", similarity)
	}
}

func TestStallDetectorDifferentOutputs(t *testing.T) {
	t.Parallel()
	d := NewStallDetector(3, 0.67)
	d.Observe("TASK-1", "execute", "output a")
	d.Observe("TASK-1", "execute", "output b")
	stalled, _ := d.Observe("TASK-1", "execute", "output c")
	if stalled {
		t.Fatal("did not expect stall for different outputs")
	}
}

func TestStallDetectorTwoOfThreeDetectsAtThreshold(t *testing.T) {
	t.Parallel()
	d := NewStallDetector(3, 0.67)
	d.Observe("TASK-1", "review", "repeat")
	d.Observe("TASK-1", "review", "repeat")
	stalled, similarity := d.Observe("TASK-1", "review", "different")
	if !stalled {
		t.Fatal("expected stall for 2/3 repeated outputs")
	}
	if similarity < 0.67 {
		t.Fatalf("expected similarity >= 0.67, got %.2f", similarity)
	}
}

func TestStallDetectorNormalizationIgnoresTimestamps(t *testing.T) {
	t.Parallel()
	d := NewStallDetector(3, 0.67)
	d.Observe("TASK-1", "execute", "finished at 2026-01-01T10:00:00Z")
	d.Observe("TASK-1", "execute", "finished at 2026-01-01T10:00:01Z")
	stalled, _ := d.Observe("TASK-1", "execute", "finished at 2026-01-01T10:00:02Z")
	if !stalled {
		t.Fatal("expected normalized timestamps to be treated as stall")
	}
}
