package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Codex is the main client for interacting with the Codex CLI.
type Codex struct {
	exec    *codexExec
	options CodexOptions
}

// New creates a Codex client.
func New(options CodexOptions) (*Codex, error) {
	exec, err := newCodexExec(options)
	if err != nil {
		return nil, err
	}
	return &Codex{
		exec:    exec,
		options: options,
	}, nil
}

// StartThread starts a new thread.
func (c *Codex) StartThread(options *ThreadOptions) *Thread {
	return newThread(c.exec, c.options, options, "")
}

// ResumeThread resumes a previously persisted thread ID.
func (c *Codex) ResumeThread(id string, options *ThreadOptions) *Thread {
	return newThread(c.exec, c.options, options, strings.TrimSpace(id))
}

// Thread represents one Codex conversation thread.
type Thread struct {
	exec       *codexExec
	codexOpts  CodexOptions
	threadOpts ThreadOptions

	idMu sync.RWMutex
	id   string
}

func newThread(exec *codexExec, codexOpts CodexOptions, options *ThreadOptions, id string) *Thread {
	var threadOpts ThreadOptions
	if options != nil {
		threadOpts = *options
	}
	return &Thread{
		exec:       exec,
		codexOpts:  codexOpts,
		threadOpts: threadOpts,
		id:         id,
	}
}

// ID returns the current thread id.
// Empty string means the first turn has not emitted thread.started yet.
func (t *Thread) ID() string {
	t.idMu.RLock()
	defer t.idMu.RUnlock()
	return t.id
}

func (t *Thread) setID(id string) {
	t.idMu.Lock()
	t.id = id
	t.idMu.Unlock()
}

// RunStreamed executes one turn and streams parsed events.
func (t *Thread) RunStreamed(ctx context.Context, input any, turnOptions *TurnOptions) (*StreamedTurn, error) {
	eventsCh := make(chan ThreadEvent, 128)
	doneCh := make(chan error, 1)

	go func() {
		err := t.runInternal(ctx, input, turnOptions, func(event ThreadEvent) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case eventsCh <- event:
				return nil
			}
		})
		close(eventsCh)
		doneCh <- err
		close(doneCh)
	}()

	return &StreamedTurn{
		Events: eventsCh,
		Done:   doneCh,
	}, nil
}

// Run executes one turn and returns the completed turn payload.
func (t *Thread) Run(ctx context.Context, input any, turnOptions *TurnOptions) (*Turn, error) {
	items := make([]ThreadItem, 0, 16)
	var finalResponse string
	var usage *Usage
	var turnFailure *ThreadError
	var streamFailure string

	err := t.runInternal(ctx, input, turnOptions, func(event ThreadEvent) error {
		switch event.Type {
		case "item.completed":
			if event.Item != nil {
				items = append(items, *event.Item)
				if event.Item.Type == "agent_message" {
					finalResponse = event.Item.Text
				}
			}
		case "turn.completed":
			usage = event.Usage
		case "turn.failed":
			turnFailure = event.Error
		case "error":
			streamFailure = strings.TrimSpace(event.Message)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if turnFailure != nil && strings.TrimSpace(turnFailure.Message) != "" {
		return nil, errors.New(turnFailure.Message)
	}
	if streamFailure != "" {
		return nil, errors.New(streamFailure)
	}

	return &Turn{
		Items:         items,
		FinalResponse: finalResponse,
		Usage:         usage,
	}, nil
}

func (t *Thread) runInternal(ctx context.Context, input any, turnOptions *TurnOptions, emit func(ThreadEvent) error) error {
	var opts TurnOptions
	if turnOptions != nil {
		opts = *turnOptions
	}

	schemaFile, err := createOutputSchemaFile(opts.OutputSchema)
	if err != nil {
		return err
	}
	defer func() {
		_ = schemaFile.cleanup()
	}()

	prompt, images, err := normalizeInput(input)
	if err != nil {
		return err
	}

	err = t.exec.run(ctx, execRunArgs{
		Input:                 prompt,
		BaseURL:               t.codexOpts.BaseURL,
		APIKey:                t.codexOpts.APIKey,
		ThreadID:              t.ID(),
		Images:                images,
		Model:                 t.threadOpts.Model,
		SandboxMode:           t.threadOpts.SandboxMode,
		WorkingDirectory:      t.threadOpts.WorkingDirectory,
		SkipGitRepoCheck:      t.threadOpts.SkipGitRepoCheck,
		OutputSchemaFile:      schemaFile.schemaPath,
		ModelReasoningEffort:  t.threadOpts.ModelReasoningEffort,
		NetworkAccessEnabled:  t.threadOpts.NetworkAccessEnabled,
		WebSearchMode:         t.threadOpts.WebSearchMode,
		WebSearchEnabled:      t.threadOpts.WebSearchEnabled,
		ApprovalPolicy:        t.threadOpts.ApprovalPolicy,
		AdditionalDirectories: t.threadOpts.AdditionalDirectories,
	}, func(line []byte) error {
		event, err := parseThreadEvent(line)
		if err != nil {
			return err
		}
		if event.Type == "thread.started" && event.ThreadID != "" {
			t.setID(event.ThreadID)
		}
		return emit(event)
	})
	if err != nil {
		return err
	}
	return nil
}

func normalizeInput(input any) (prompt string, images []string, err error) {
	switch in := input.(type) {
	case string:
		return in, nil, nil
	case []UserInput:
		promptParts := make([]string, 0, len(in))
		images := make([]string, 0, len(in))
		for _, item := range in {
			switch item.Type {
			case UserInputTypeText:
				promptParts = append(promptParts, item.Text)
			case UserInputTypeLocalImage:
				images = append(images, item.Path)
			default:
				return "", nil, fmt.Errorf("unsupported user input type: %s", item.Type)
			}
		}
		return strings.Join(promptParts, "\n\n"), images, nil
	default:
		return "", nil, errors.New("input must be string or []UserInput")
	}
}

func parseThreadEvent(line []byte) (ThreadEvent, error) {
	var base struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &base); err != nil {
		return ThreadEvent{}, fmt.Errorf("parse thread event envelope: %w", err)
	}

	event := ThreadEvent{
		Type: base.Type,
		Raw:  append(json.RawMessage(nil), line...),
	}

	switch base.Type {
	case "thread.started":
		var payload struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			return ThreadEvent{}, fmt.Errorf("parse thread.started event: %w", err)
		}
		event.ThreadID = payload.ThreadID
	case "turn.completed":
		var payload struct {
			Type  string `json:"type"`
			Usage Usage  `json:"usage"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			return ThreadEvent{}, fmt.Errorf("parse turn.completed event: %w", err)
		}
		event.Usage = &payload.Usage
	case "turn.failed":
		var payload struct {
			Type  string      `json:"type"`
			Error ThreadError `json:"error"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			return ThreadEvent{}, fmt.Errorf("parse turn.failed event: %w", err)
		}
		event.Error = &payload.Error
	case "item.started", "item.updated", "item.completed":
		var payload struct {
			Type string          `json:"type"`
			Item json.RawMessage `json:"item"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			return ThreadEvent{}, fmt.Errorf("parse %s event: %w", base.Type, err)
		}
		var item ThreadItem
		if err := json.Unmarshal(payload.Item, &item); err != nil {
			return ThreadEvent{}, fmt.Errorf("parse item payload: %w", err)
		}
		item.Raw = append(json.RawMessage(nil), payload.Item...)
		event.Item = &item
	case "error":
		var payload struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(line, &payload); err != nil {
			return ThreadEvent{}, fmt.Errorf("parse error event: %w", err)
		}
		event.Message = payload.Message
	}

	return event, nil
}
