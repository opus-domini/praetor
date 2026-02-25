package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Query represents an active Claude Code stream session.
type Query struct {
	transport *processTransport

	msgCh chan SDKMessage
	errCh chan error
	done  chan struct{}

	onControlRequest ControlRequestHandler
	canUseTool       CanUseTool
	onMCPMessage     OnMCPMessage

	pendingMu sync.Mutex
	pending   map[string]chan controlResponse

	incomingCancelMu sync.Mutex
	incomingCancel   map[string]context.CancelFunc

	hookMu        sync.Mutex
	hookCallbacks map[string]HookCallback
	nextHookID    int

	initDone   chan struct{}
	initMu     sync.Mutex
	initErr    error
	initResult *InitializeResponse

	closeOnce sync.Once
}

// NewQuery starts a new Claude Code process and begins async initialization.
func NewQuery(ctx context.Context, opts Options) (*Query, error) {
	transport, err := newProcessTransport(ctx, opts)
	if err != nil {
		return nil, err
	}

	q := &Query{
		transport:        transport,
		msgCh:            make(chan SDKMessage, 256),
		errCh:            make(chan error, 16),
		done:             make(chan struct{}),
		onControlRequest: opts.OnControlRequest,
		canUseTool:       opts.CanUseTool,
		onMCPMessage:     opts.OnMCPMessage,
		pending:          make(map[string]chan controlResponse),
		incomingCancel:   make(map[string]context.CancelFunc),
		hookCallbacks:    make(map[string]HookCallback),
		initDone:         make(chan struct{}),
	}

	go q.readLoop()
	go q.initializeAsync(opts)
	return q, nil
}

// Messages streams SDK messages from the CLI process.
func (q *Query) Messages() <-chan SDKMessage {
	return q.msgCh
}

// Errors streams non-fatal runtime errors.
func (q *Query) Errors() <-chan error {
	return q.errCh
}

// Close terminates the query and underlying process.
func (q *Query) Close() error {
	var closeErr error
	q.closeOnce.Do(func() {
		close(q.done)

		q.pendingMu.Lock()
		for id, ch := range q.pending {
			close(ch)
			delete(q.pending, id)
		}
		q.pendingMu.Unlock()

		q.incomingCancelMu.Lock()
		for id, cancel := range q.incomingCancel {
			cancel()
			delete(q.incomingCancel, id)
		}
		q.incomingCancelMu.Unlock()

		closeErr = q.transport.close()
		close(q.msgCh)
		close(q.errCh)
	})
	return closeErr
}

// WaitInitialized waits for async initialize request completion.
func (q *Query) WaitInitialized(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.done:
		return errors.New("query closed")
	case <-q.initDone:
		q.initMu.Lock()
		defer q.initMu.Unlock()
		return q.initErr
	}
}

// InitializationResult returns initialization metadata.
func (q *Query) InitializationResult(ctx context.Context) (*InitializeResponse, error) {
	if err := q.WaitInitialized(ctx); err != nil {
		return nil, err
	}
	q.initMu.Lock()
	defer q.initMu.Unlock()
	if q.initResult == nil {
		return nil, errors.New("initialize response not available")
	}
	c := *q.initResult
	return &c, nil
}

// SupportedCommands returns slash command metadata from initialization.
func (q *Query) SupportedCommands(ctx context.Context) ([]SlashCommand, error) {
	init, err := q.InitializationResult(ctx)
	if err != nil {
		return nil, err
	}
	return append([]SlashCommand(nil), init.Commands...), nil
}

// SupportedModels returns model metadata from initialization.
func (q *Query) SupportedModels(ctx context.Context) ([]ModelInfo, error) {
	init, err := q.InitializationResult(ctx)
	if err != nil {
		return nil, err
	}
	return append([]ModelInfo(nil), init.Models...), nil
}

// AccountInfo returns authenticated account metadata from initialization.
func (q *Query) AccountInfo(ctx context.Context) (AccountInfo, error) {
	init, err := q.InitializationResult(ctx)
	if err != nil {
		return AccountInfo{}, err
	}
	return init.Account, nil
}

// EndInput closes stdin for the current query.
func (q *Query) EndInput() error {
	return q.transport.endInput()
}

// SendUserMessage writes a user message envelope to the query stream.
func (q *Query) SendUserMessage(msg UserTextMessage) error {
	return q.transport.writeJSONLine(msg)
}

// SendUserText sends a single user text message into the session.
func (q *Query) SendUserText(text string) error {
	envelope := UserTextMessage{
		Type:      "user",
		SessionID: "",
		Message: UserMessage{
			Role: "user",
			Content: []UserTextContent{
				{Type: "text", Text: text},
			},
		},
		ParentToolUseID: nil,
	}
	return q.SendUserMessage(envelope)
}

// StreamInput sends user messages from a channel until closed or context canceled.
func (q *Query) StreamInput(ctx context.Context, input <-chan UserTextMessage, endOnClose bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-q.done:
			return errors.New("query closed")
		case msg, ok := <-input:
			if !ok {
				if endOnClose {
					return q.EndInput()
				}
				return nil
			}
			if err := q.SendUserMessage(msg); err != nil {
				return err
			}
		}
	}
}

// Interrupt interrupts the current task execution.
func (q *Query) Interrupt(ctx context.Context) error {
	_, err := q.request(ctx, map[string]any{"subtype": "interrupt"})
	return err
}

// SetPermissionMode changes permission mode for subsequent tool calls.
func (q *Query) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	_, err := q.request(ctx, map[string]any{
		"subtype": "set_permission_mode",
		"mode":    string(mode),
	})
	return err
}

// SetModel sets the active model for subsequent responses.
func (q *Query) SetModel(ctx context.Context, model string) error {
	_, err := q.request(ctx, map[string]any{
		"subtype": "set_model",
		"model":   model,
	})
	return err
}

// SetMaxThinkingTokens sets max thinking tokens.
func (q *Query) SetMaxThinkingTokens(ctx context.Context, tokens *int) error {
	var value any
	if tokens != nil {
		value = *tokens
	}
	_, err := q.request(ctx, map[string]any{
		"subtype":             "set_max_thinking_tokens",
		"max_thinking_tokens": value,
	})
	return err
}

// ApplyFlagSettings sends apply_flag_settings control request.
func (q *Query) ApplyFlagSettings(ctx context.Context, settings map[string]any) error {
	_, err := q.request(ctx, map[string]any{
		"subtype":  "apply_flag_settings",
		"settings": settings,
	})
	return err
}

// StopTask requests task stop for async task IDs.
func (q *Query) StopTask(ctx context.Context, taskID string) error {
	_, err := q.request(ctx, map[string]any{
		"subtype": "stop_task",
		"task_id": taskID,
	})
	return err
}

// RewindFiles rewinds tracked files to a user message.
func (q *Query) RewindFiles(ctx context.Context, userMessageID string, dryRun bool) (*RewindFilesResult, error) {
	respRaw, err := q.request(ctx, map[string]any{
		"subtype":         "rewind_files",
		"user_message_id": userMessageID,
		"dry_run":         dryRun,
	})
	if err != nil {
		return nil, err
	}

	var out RewindFilesResult
	if len(respRaw) > 0 {
		if err := json.Unmarshal(respRaw, &out); err != nil {
			return nil, fmt.Errorf("decode rewind_files response: %w", err)
		}
	}
	return &out, nil
}

// ReconnectMCPServer requests reconnect for a configured MCP server.
func (q *Query) ReconnectMCPServer(ctx context.Context, serverName string) error {
	_, err := q.request(ctx, map[string]any{
		"subtype":    "mcp_reconnect",
		"serverName": serverName,
	})
	return err
}

// ToggleMCPServer enables or disables an MCP server by name.
func (q *Query) ToggleMCPServer(ctx context.Context, serverName string, enabled bool) error {
	_, err := q.request(ctx, map[string]any{
		"subtype":    "mcp_toggle",
		"serverName": serverName,
		"enabled":    enabled,
	})
	return err
}

// MCPAuthenticate requests auth flow for a server that needs auth.
func (q *Query) MCPAuthenticate(ctx context.Context, serverName string) (json.RawMessage, error) {
	return q.request(ctx, map[string]any{
		"subtype":    "mcp_authenticate",
		"serverName": serverName,
	})
}

// MCPClearAuth clears saved auth for a server.
func (q *Query) MCPClearAuth(ctx context.Context, serverName string) (json.RawMessage, error) {
	return q.request(ctx, map[string]any{
		"subtype":    "mcp_clear_auth",
		"serverName": serverName,
	})
}

// MCPServerStatus returns status for configured MCP servers.
func (q *Query) MCPServerStatus(ctx context.Context) ([]MCPServerStatus, error) {
	respRaw, err := q.request(ctx, map[string]any{"subtype": "mcp_status"})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		MCPServers []MCPServerStatus `json:"mcpServers"`
	}
	if err := json.Unmarshal(respRaw, &wrapper); err != nil {
		return nil, fmt.Errorf("decode mcp_status response: %w", err)
	}
	return wrapper.MCPServers, nil
}

// SetMCPServers replaces dynamically managed MCP servers.
func (q *Query) SetMCPServers(ctx context.Context, servers map[string]MCPServerConfigForProcessTransport) (*MCPSetServersResult, error) {
	respRaw, err := q.request(ctx, map[string]any{
		"subtype": "mcp_set_servers",
		"servers": servers,
	})
	if err != nil {
		return nil, err
	}
	var out MCPSetServersResult
	if err := json.Unmarshal(respRaw, &out); err != nil {
		return nil, fmt.Errorf("decode mcp_set_servers response: %w", err)
	}
	return &out, nil
}

// AwaitResult waits until a "result" SDK message is received.
func (q *Query) AwaitResult(ctx context.Context) (SDKMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return SDKMessage{}, ctx.Err()
		case <-q.done:
			return SDKMessage{}, errors.New("query closed")
		case msg, ok := <-q.msgCh:
			if !ok {
				return SDKMessage{}, errors.New("message stream closed")
			}
			if msg.Type == "result" {
				return msg, nil
			}
		}
	}
}

// ParseResultMessage decodes SDKMessage payload into ResultMessage.
func ParseResultMessage(msg SDKMessage) (*ResultMessage, error) {
	var out ResultMessage
	if err := json.Unmarshal(msg.Raw, &out); err != nil {
		return nil, err
	}
	out.Raw = append(json.RawMessage(nil), msg.Raw...)
	return &out, nil
}

func (q *Query) initialize(ctx context.Context, opts Options) error {
	hooksPayload := q.buildInitializeHooks(opts.Hooks)
	req := initializeRequest{
		Subtype:            "initialize",
		Hooks:              hooksPayload,
		JSONSchema:         opts.JSONSchema,
		SystemPrompt:       opts.SystemPrompt,
		AppendSystemPrompt: opts.AppendSystemPrompt,
		Agents:             opts.Agents,
		PromptSuggestions:  opts.PromptSuggestions,
	}
	respRaw, err := q.request(ctx, req)
	if err != nil {
		return err
	}
	var initResp InitializeResponse
	if err := json.Unmarshal(respRaw, &initResp); err != nil {
		return fmt.Errorf("decode initialize response: %w", err)
	}

	q.initMu.Lock()
	q.initResult = &initResp
	q.initMu.Unlock()
	return nil
}

func (q *Query) buildInitializeHooks(hooks map[HookEvent][]HookCallbackMatcher) map[HookEvent][]sdkHookCallbackMatcher {
	if len(hooks) == 0 {
		return nil
	}
	out := make(map[HookEvent][]sdkHookCallbackMatcher, len(hooks))
	for event, matchers := range hooks {
		if len(matchers) == 0 {
			continue
		}
		payloadMatchers := make([]sdkHookCallbackMatcher, 0, len(matchers))
		for _, m := range matchers {
			if len(m.Hooks) == 0 {
				continue
			}
			callbackIDs := make([]string, 0, len(m.Hooks))
			q.hookMu.Lock()
			for _, cb := range m.Hooks {
				cbID := fmt.Sprintf("hook_%d", q.nextHookID)
				q.nextHookID++
				q.hookCallbacks[cbID] = cb
				callbackIDs = append(callbackIDs, cbID)
			}
			q.hookMu.Unlock()
			payloadMatchers = append(payloadMatchers, sdkHookCallbackMatcher{
				Matcher:         m.Matcher,
				HookCallbackIDs: callbackIDs,
				Timeout:         m.Timeout,
			})
		}
		if len(payloadMatchers) > 0 {
			out[event] = payloadMatchers
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (q *Query) request(ctx context.Context, request any) (json.RawMessage, error) {
	reqID, err := randomRequestID()
	if err != nil {
		return nil, err
	}

	respCh := make(chan controlResponse, 1)
	q.pendingMu.Lock()
	q.pending[reqID] = respCh
	q.pendingMu.Unlock()

	envelope := controlRequestEnvelope{
		Type:      "control_request",
		RequestID: reqID,
		Request:   request,
	}
	if err := q.transport.writeJSONLine(envelope); err != nil {
		q.pendingMu.Lock()
		delete(q.pending, reqID)
		q.pendingMu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		q.pendingMu.Lock()
		delete(q.pending, reqID)
		q.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-q.done:
		return nil, errors.New("query closed")
	case resp, ok := <-respCh:
		if !ok {
			return nil, errors.New("query closed before control response")
		}
		if resp.Subtype == "success" {
			return resp.Response, nil
		}
		if len(resp.PendingPermissionRequests) > 0 {
			for _, pending := range resp.PendingPermissionRequests {
				go q.handleControlRequest(pending)
			}
		}
		if resp.Error == "" {
			return nil, errors.New("control request failed")
		}
		return nil, errors.New(resp.Error)
	}
}

func (q *Query) readLoop() {
	err := q.transport.readLines(func(line []byte) error {
		var base struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &base); err != nil {
			return fmt.Errorf("decode envelope: %w", err)
		}

		switch base.Type {
		case "control_response":
			var env controlResponseEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				return fmt.Errorf("decode control_response: %w", err)
			}
			q.pendingMu.Lock()
			ch := q.pending[env.Response.RequestID]
			if ch != nil {
				delete(q.pending, env.Response.RequestID)
				ch <- env.Response
				close(ch)
			}
			q.pendingMu.Unlock()
			return nil
		case "control_request":
			var env incomingControlRequestEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				return fmt.Errorf("decode control_request: %w", err)
			}
			go q.handleControlRequest(env)
			return nil
		case "control_cancel_request":
			var env incomingControlCancelRequestEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				return fmt.Errorf("decode control_cancel_request: %w", err)
			}
			q.cancelIncomingControlRequest(env.RequestID)
			return nil
		case "keep_alive", "streamlined_text", "streamlined_tool_use_summary":
			return nil
		default:
			msg := SDKMessage{
				Type: base.Type,
				Raw:  append(json.RawMessage(nil), line...),
			}
			select {
			case <-q.done:
				return nil
			case q.msgCh <- msg:
				return nil
			}
		}
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		q.pushErr(err)
	}
	_ = q.Close()
}

func (q *Query) cancelIncomingControlRequest(requestID string) {
	q.incomingCancelMu.Lock()
	cancel := q.incomingCancel[requestID]
	delete(q.incomingCancel, requestID)
	q.incomingCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (q *Query) handleControlRequest(env incomingControlRequestEnvelope) {
	ctx, cancel := context.WithCancel(context.Background())
	q.incomingCancelMu.Lock()
	q.incomingCancel[env.RequestID] = cancel
	q.incomingCancelMu.Unlock()
	defer func() {
		cancel()
		q.incomingCancelMu.Lock()
		delete(q.incomingCancel, env.RequestID)
		q.incomingCancelMu.Unlock()
	}()

	var subtype requestSubtypeOnly
	if err := json.Unmarshal(env.Request, &subtype); err != nil {
		_ = q.respondControlError(env.RequestID, fmt.Errorf("decode request subtype: %w", err))
		return
	}

	switch subtype.Subtype {
	case "can_use_tool":
		if err := q.handleCanUseTool(ctx, env.RequestID, env.Request); err != nil {
			_ = q.respondControlError(env.RequestID, err)
		}
		return
	case "hook_callback":
		if err := q.handleHookCallback(ctx, env.RequestID, env.Request); err != nil {
			_ = q.respondControlError(env.RequestID, err)
		}
		return
	case "mcp_message":
		if err := q.handleMCPMessage(ctx, env.RequestID, env.Request); err != nil {
			_ = q.respondControlError(env.RequestID, err)
		}
		return
	}

	if q.onControlRequest == nil {
		_ = q.respondControlError(env.RequestID, fmt.Errorf("unsupported control request subtype: %s", subtype.Subtype))
		return
	}
	req := IncomingControlRequest{
		RequestID: env.RequestID,
		Subtype:   subtype.Subtype,
		Raw:       env.Request,
	}
	response, err := q.onControlRequest(ctx, req)
	if err != nil {
		_ = q.respondControlError(env.RequestID, err)
		return
	}
	_ = q.respondControlSuccess(env.RequestID, response)
}

func (q *Query) handleCanUseTool(ctx context.Context, requestID string, raw json.RawMessage) error {
	if q.canUseTool == nil {
		return errors.New("canUseTool callback is not configured")
	}
	var req canUseToolControlRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("decode can_use_tool request: %w", err)
	}

	resp, err := q.canUseTool(ctx, CanUseToolRequest{
		ToolName:              req.ToolName,
		Input:                 req.Input,
		PermissionSuggestions: req.PermissionSuggestions,
		BlockedPath:           req.BlockedPath,
		DecisionReason:        req.DecisionReason,
		ToolUseID:             req.ToolUseID,
		AgentID:               req.AgentID,
	})
	if err != nil {
		return err
	}
	if resp.ToolUseID == "" {
		resp.ToolUseID = req.ToolUseID
	}
	return q.respondControlSuccess(requestID, resp)
}

func (q *Query) handleHookCallback(ctx context.Context, requestID string, raw json.RawMessage) error {
	var req hookCallbackControlRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("decode hook_callback request: %w", err)
	}

	q.hookMu.Lock()
	cb := q.hookCallbacks[req.CallbackID]
	q.hookMu.Unlock()
	if cb == nil {
		return fmt.Errorf("hook callback not found: %s", req.CallbackID)
	}
	response, err := cb(ctx, req.Input, req.ToolUseID)
	if err != nil {
		return err
	}
	return q.respondControlSuccess(requestID, response)
}

func (q *Query) handleMCPMessage(ctx context.Context, requestID string, raw json.RawMessage) error {
	if q.onMCPMessage == nil {
		return errors.New("mcp_message handler is not configured")
	}
	var req mcpMessageControlRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("decode mcp_message request: %w", err)
	}
	resp, err := q.onMCPMessage(ctx, MCPMessageRequest{
		ServerName: req.ServerName,
		Message:    req.Message,
	})
	if err != nil {
		return err
	}
	if len(resp) == 0 {
		resp = json.RawMessage(`{"jsonrpc":"2.0","result":{},"id":0}`)
	}
	return q.respondControlSuccess(requestID, map[string]any{
		"mcp_response": resp,
	})
}

func (q *Query) respondControlSuccess(requestID string, response any) error {
	env := outgoingControlResponseEnvelope{
		Type: "control_response",
		Response: outgoingControlResponse{
			Subtype:   "success",
			RequestID: requestID,
			Response:  response,
		},
	}
	return q.transport.writeJSONLine(env)
}

func (q *Query) respondControlError(requestID string, err error) error {
	env := outgoingControlResponseEnvelope{
		Type: "control_response",
		Response: outgoingControlResponse{
			Subtype:   "error",
			RequestID: requestID,
			Error:     err.Error(),
		},
	}
	return q.transport.writeJSONLine(env)
}

func randomRequestID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate request id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (q *Query) initializeAsync(opts Options) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	err := q.initialize(ctx, opts)
	q.initMu.Lock()
	q.initErr = err
	q.initMu.Unlock()
	close(q.initDone)
	if err != nil {
		q.pushErr(fmt.Errorf("initialize failed: %w", err))
	}
}

func (q *Query) pushErr(err error) {
	if err == nil {
		return
	}
	select {
	case <-q.done:
		return
	default:
	}
	select {
	case <-q.done:
	case q.errCh <- err:
	default:
	}
}
