package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	lockSchemaVersion = 2
)

type lockMeta struct {
	Version   int    `json:"version"`
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
	StartedAt string `json:"started_at"`
	Token     string `json:"token"`
	Runtime   string `json:"runtime_key"`
}

// AcquireRunLock acquires a lock for one plan run.
func (s *Store) AcquireRunLock(planFile string, force bool) (RunLock, error) {
	if err := s.Init(); err != nil {
		return RunLock{}, err
	}

	runtimeKey := s.RuntimeKey(planFile)
	lockPath := s.LockFile(planFile)
	hostname, _ := os.Hostname()

	for range 4 {
		token, err := randomHex(12)
		if err != nil {
			return RunLock{}, err
		}
		meta := lockMeta{
			Version:   lockSchemaVersion,
			PID:       os.Getpid(),
			Hostname:  strings.TrimSpace(hostname),
			StartedAt: time.Now().UTC().Format(time.RFC3339),
			Token:     token,
			Runtime:   runtimeKey,
		}
		payload, err := json.Marshal(meta)
		if err != nil {
			return RunLock{}, fmt.Errorf("encode lock metadata: %w", err)
		}
		payload = append(payload, '\n')

		file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if _, writeErr := file.Write(payload); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return RunLock{}, fmt.Errorf("write lock file: %w", writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(lockPath)
				return RunLock{}, fmt.Errorf("close lock file: %w", closeErr)
			}
			return RunLock{Path: lockPath, Token: token}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return RunLock{}, fmt.Errorf("open lock file: %w", err)
		}

		data, readErr := os.ReadFile(lockPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return RunLock{}, fmt.Errorf("read lock file: %w", readErr)
		}
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}

		existing := parseLockFile(data)
		if lockIsActive(existing, hostname, runtimeKey) && !force {
			return RunLock{}, fmt.Errorf("plan is already running (pid=%d, started=%s); use --force to override", existing.PID, existing.StartedAt)
		}
		if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return RunLock{}, fmt.Errorf("remove stale lock file: %w", removeErr)
		}
	}
	return RunLock{}, errors.New("unable to acquire lock after multiple attempts")
}

// ReleaseRunLock releases a plan lock.
func (s *Store) ReleaseRunLock(lock RunLock) error {
	lockPath := strings.TrimSpace(lock.Path)
	if lockPath == "" {
		return nil
	}

	data, err := os.ReadFile(lockPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}
	existing := parseLockFile(data)
	if existing.Token == "" {
		return errors.New("lock file does not contain ownership token")
	}
	if strings.TrimSpace(lock.Token) == "" || existing.Token != strings.TrimSpace(lock.Token) {
		return fmt.Errorf("lock ownership mismatch for %s", lockPath)
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove lock file: %w", err)
	}
	return nil
}

func parseLockFile(data []byte) lockMeta {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return lockMeta{}
	}

	var meta lockMeta
	if err := json.Unmarshal([]byte(trimmed), &meta); err == nil {
		return meta
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 {
		return lockMeta{}
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	started := ""
	if len(lines) > 1 {
		started = strings.TrimSpace(lines[1])
	}
	return lockMeta{
		PID:       pid,
		StartedAt: started,
	}
}

func lockIsActive(meta lockMeta, hostname, runtimeKey string) bool {
	if meta.PID <= 0 {
		return false
	}

	localHostname := strings.TrimSpace(hostname)
	lockHostname := strings.TrimSpace(meta.Hostname)
	if lockHostname != "" && localHostname != "" && !strings.EqualFold(lockHostname, localHostname) {
		return false
	}

	expectedRuntime := strings.TrimSpace(runtimeKey)
	lockRuntime := strings.TrimSpace(meta.Runtime)
	if lockRuntime != "" && expectedRuntime != "" && lockRuntime != expectedRuntime {
		return false
	}

	return processIsRunning(meta.PID)
}

func processIsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

func randomHex(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("random size must be positive")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
