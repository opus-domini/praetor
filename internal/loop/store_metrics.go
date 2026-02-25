package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// CostEntry records one agent invocation's metrics.
type CostEntry struct {
	Timestamp string
	RunID     string
	TaskID    string
	Agent     string
	Role      string
	DurationS float64
	Status    string
	CostUSD   float64
}

// WriteTaskMetrics appends one cost entry to the tracking ledger.
func (s *Store) WriteTaskMetrics(entry CostEntry) error {
	path := filepath.Join(s.CostsDir(), "tracking.tsv")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open cost tracking file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock cost tracking file: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat cost tracking file: %w", err)
	}
	if info.Size() == 0 {
		if _, err := fmt.Fprint(f, "timestamp\trun_id\ttask_id\tagent\trole\tduration_s\tstatus\tcost_usd\n"); err != nil {
			return fmt.Errorf("write cost header: %w", err)
		}
	}

	if _, err := fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\t%.2f\t%s\t%.6f\n",
		entry.Timestamp, entry.RunID, entry.TaskID, entry.Agent, entry.Role,
		entry.DurationS, entry.Status, entry.CostUSD); err != nil {
		return fmt.Errorf("write cost entry: %w", err)
	}
	return nil
}

// CheckpointEntry records one state transition in the audit log.
type CheckpointEntry struct {
	Timestamp string
	Status    string
	TaskID    string
	Signature string
	RunID     string
	Message   string
}

// WriteCheckpoint appends to the history log and overwrites the current checkpoint.
func (s *Store) WriteCheckpoint(planFile string, entry CheckpointEntry) error {
	historyPath := filepath.Join(s.CheckpointsDir(), "history.tsv")
	f, err := os.OpenFile(historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open checkpoint history: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock checkpoint history: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat checkpoint history: %w", err)
	}
	if info.Size() == 0 {
		if _, err := fmt.Fprint(f, "timestamp\tstatus\ttask_id\tsignature\trun_id\tmessage\n"); err != nil {
			return fmt.Errorf("write checkpoint header: %w", err)
		}
	}

	if _, err := fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\t%s\n",
		entry.Timestamp, entry.Status, entry.TaskID, entry.Signature,
		entry.RunID, entry.Message); err != nil {
		return fmt.Errorf("write checkpoint entry: %w", err)
	}

	currentPath := s.currentCheckpointFile(planFile)
	content := fmt.Sprintf("timestamp=%s\nstatus=%s\ntask_id=%s\nsignature=%s\nrun_id=%s\nmessage=%s\n",
		entry.Timestamp, entry.Status, entry.TaskID, entry.Signature,
		entry.RunID, entry.Message)
	if err := os.WriteFile(currentPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write current checkpoint: %w", err)
	}
	return nil
}
