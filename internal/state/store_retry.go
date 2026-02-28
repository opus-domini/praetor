package state

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opus-domini/praetor/internal/domain"
)

// TaskSignature returns a stable signature used by retries and feedback files.
func TaskSignature(taskKey string) string {
	hash := sha256.Sum256([]byte(taskKey))
	return hex.EncodeToString(hash[:])
}

func taskSignatureKey(index int, task domain.StateTask) string {
	if strings.TrimSpace(task.ID) != "" {
		return "id:" + strings.TrimSpace(task.ID)
	}
	return fmt.Sprintf("index:%d:title:%s", index, strings.TrimSpace(task.Title))
}

// TaskKey builds the signature key for a task.
func TaskKey(index int, task domain.StateTask) string {
	return taskSignatureKey(index, task)
}

// TaskSignatureForPlan returns the stable, plan-scoped signature for retries/feedback.
func (s *Store) TaskSignatureForPlan(slug string, index int, task domain.StateTask) string {
	scope := s.RuntimeKey(slug) + "|" + taskSignatureKey(index, task)
	return TaskSignature(scope)
}

// ReadRetryCount reads current retry count for one task signature.
func (s *Store) ReadRetryCount(signature string) (int, error) {
	path := filepath.Join(s.RetriesDir(), signature+".count")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read retry file: %w", err)
	}

	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse retry count for %s: %w", signature, err)
	}
	if value < 0 {
		return 0, nil
	}
	return value, nil
}

// IncrementRetryCount increments retry count and returns the new value.
func (s *Store) IncrementRetryCount(signature string) (int, error) {
	count, err := s.ReadRetryCount(signature)
	if err != nil {
		return 0, err
	}
	count++

	path := filepath.Join(s.RetriesDir(), signature+".count")
	if err := os.WriteFile(path, []byte(strconv.Itoa(count)), 0o644); err != nil {
		return 0, fmt.Errorf("write retry file: %w", err)
	}
	return count, nil
}

// ClearRetryCount deletes a retry counter file.
func (s *Store) ClearRetryCount(signature string) error {
	path := filepath.Join(s.RetriesDir(), signature+".count")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove retry file: %w", err)
	}
	return nil
}

// ReadFeedback reads previous reviewer feedback for a task signature.
func (s *Store) ReadFeedback(signature string) (string, error) {
	path := filepath.Join(s.FeedbackDir(), signature+".txt")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read feedback file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteFeedback persists reviewer feedback for a task signature.
func (s *Store) WriteFeedback(signature, feedback string) error {
	path := filepath.Join(s.FeedbackDir(), signature+".txt")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(feedback)), 0o644); err != nil {
		return fmt.Errorf("write feedback file: %w", err)
	}
	return nil
}

// ClearFeedback deletes a feedback file.
func (s *Store) ClearFeedback(signature string) error {
	path := filepath.Join(s.FeedbackDir(), signature+".txt")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove feedback file: %w", err)
	}
	return nil
}
