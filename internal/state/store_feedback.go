package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/opus-domini/praetor/internal/domain"
)

func (s *Store) feedbackLogPath(slug, signature string) string {
	return filepath.Join(s.FeedbackDir(), strings.TrimSpace(slug), strings.TrimSpace(signature)+".jsonl")
}

// AppendTaskFeedback appends one structured feedback entry for a task.
func (s *Store) AppendTaskFeedback(slug, signature string, fb domain.TaskFeedback) error {
	path := s.feedbackLogPath(slug, signature)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create feedback directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open feedback log: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock feedback log: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	encoded, err := json.Marshal(fb)
	if err != nil {
		return fmt.Errorf("encode feedback: %w", err)
	}
	encoded = append(encoded, '\n')
	if _, err := f.Write(encoded); err != nil {
		return fmt.Errorf("append feedback: %w", err)
	}
	return nil
}

// LoadTaskFeedback returns structured feedback entries sorted by attempt/timestamp.
func (s *Store) LoadTaskFeedback(slug, signature string) ([]domain.TaskFeedback, error) {
	path := s.feedbackLogPath(slug, signature)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open feedback log: %w", err)
	}
	defer func() { _ = f.Close() }()

	feedback := make([]domain.TaskFeedback, 0)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 16*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item domain.TaskFeedback
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode feedback entry: %w", err)
		}
		feedback = append(feedback, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan feedback log: %w", err)
	}

	sort.Slice(feedback, func(i, j int) bool {
		if feedback[i].Attempt == feedback[j].Attempt {
			return feedback[i].Timestamp < feedback[j].Timestamp
		}
		return feedback[i].Attempt < feedback[j].Attempt
	})
	return feedback, nil
}
