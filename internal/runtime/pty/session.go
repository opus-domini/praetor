package pty

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// StreamSource identifies where a stream event originated.
type StreamSource string

const (
	StreamStdout StreamSource = "stdout"
	StreamStderr StreamSource = "stderr"
)

// StreamEvent is one chunk of streamed process output.
type StreamEvent struct {
	Source    StreamSource
	Data      string
	Timestamp time.Time
}

// CommandSpec defines one PTY-backed command invocation.
type CommandSpec struct {
	Args []string
	Env  []string
	Dir  string
}

// Session defines an interactive PTY session.
type Session interface {
	Start(ctx context.Context, spec CommandSpec) error
	Events() <-chan StreamEvent
	Write(input string) error
	CloseInput() error
	Wait() error
	ExitCode() int
	Close() error
}

// scriptSession uses the `script` utility to provide PTY semantics.
type scriptSession struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	events   chan StreamEvent
	waitErr  error
	waitDone chan struct{}
	exitCode atomic.Int64

	started     bool
	inputClosed bool
}

// NewScriptSession creates a PTY session backed by `script -qefc`.
func NewScriptSession() Session {
	s := &scriptSession{
		events:   make(chan StreamEvent, 64),
		waitDone: make(chan struct{}),
	}
	s.exitCode.Store(0)
	return s
}

func (s *scriptSession) Start(ctx context.Context, spec CommandSpec) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return errors.New("pty session already started")
	}
	if len(spec.Args) == 0 {
		return errors.New("pty command is required")
	}
	if _, err := exec.LookPath("script"); err != nil {
		return errors.New("pty session requires `script` command in PATH")
	}

	cmdLine := buildCommandLine(spec.Args)
	cmd := exec.CommandContext(ctx, "script", "-qefc", cmdLine, "/dev/null")
	if strings.TrimSpace(spec.Dir) != "" {
		cmd.Dir = spec.Dir
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create pty stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create pty stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create pty stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start pty session: %w", err)
	}

	s.cmd = cmd
	s.stdin = stdin
	s.started = true

	go s.readStream(stdout, StreamStdout)
	go s.readStream(stderr, StreamStderr)
	go s.waitProcess()
	return nil
}

func (s *scriptSession) Events() <-chan StreamEvent {
	return s.events
}

func (s *scriptSession) Write(input string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.stdin == nil {
		return errors.New("pty session not started")
	}
	if s.inputClosed {
		return errors.New("pty stdin already closed")
	}
	if input == "" {
		return nil
	}
	_, err := io.WriteString(s.stdin, input)
	if err != nil {
		return fmt.Errorf("write pty stdin: %w", err)
	}
	return nil
}

func (s *scriptSession) CloseInput() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stdin == nil || s.inputClosed {
		return nil
	}
	s.inputClosed = true
	if err := s.stdin.Close(); err != nil {
		return fmt.Errorf("close pty stdin: %w", err)
	}
	return nil
}

func (s *scriptSession) Wait() error {
	<-s.waitDone
	return s.waitErr
}

func (s *scriptSession) ExitCode() int {
	return int(s.exitCode.Load())
}

func (s *scriptSession) Close() error {
	_ = s.CloseInput()

	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill pty process: %w", err)
		}
	}
	return nil
}

func (s *scriptSession) readStream(reader io.Reader, source StreamSource) {
	buf := bufio.NewReader(reader)
	for {
		chunk, err := buf.ReadString('\n')
		if chunk != "" {
			s.events <- StreamEvent{
				Source:    source,
				Data:      chunk,
				Timestamp: time.Now().UTC(),
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if strings.TrimSpace(chunk) == "" {
				return
			}
		}
	}
}

func (s *scriptSession) waitProcess() {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil {
		close(s.events)
		close(s.waitDone)
		s.waitErr = errors.New("pty process not initialized")
		return
	}

	err := cmd.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			s.exitCode.Store(int64(exitErr.ExitCode()))
		} else {
			s.waitErr = err
		}
	}
	if s.exitCode.Load() == 0 {
		s.exitCode.Store(0)
	}

	close(s.events)
	close(s.waitDone)
}

func buildCommandLine(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
