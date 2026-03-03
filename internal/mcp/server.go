package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Server implements an MCP server over stdio (JSON-RPC 2.0, one message per line).
type Server struct {
	tools       *toolRegistry
	resources   *resourceRegistry
	projectDir  string
	initialized bool
}

// NewServer creates a new MCP server for the given project directory.
func NewServer(projectDir string) *Server {
	s := &Server{
		tools:      newToolRegistry(),
		resources:  newResourceRegistry(),
		projectDir: projectDir,
	}
	registerPlanTools(s)
	registerStateTools(s)
	registerConfigTools(s)
	registerExecTools(s)
	registerResources(s)
	return s
}

// Run starts the server, reading JSON-RPC messages from in and writing responses to out.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error: &jsonRPCError{
					Code:    -32700,
					Message: "Parse error",
					Data:    err.Error(),
				},
			}
			if err := enc.Encode(resp); err != nil {
				return fmt.Errorf("write parse error response: %w", err)
			}
			continue
		}

		resp := s.dispatch(req)
		if resp == nil {
			// Notification — no response needed.
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}

	return scanner.Err()
}

func (s *Server) dispatch(req jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		s.initialized = true
		return nil // notification, no response
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/read":
		return s.handleResourcesRead(req)
	case "ping":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) handleInitialize(req jsonRPCRequest) *jsonRPCResponse {
	result := initializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: serverCapabilities{
			Tools:     &toolsCapability{},
			Resources: &resourcesCapability{},
		},
		ServerInfo: serverInfo{
			Name:    "praetor",
			Version: "0.1.0",
		},
	}
	return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) handleToolsList(req jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  toolsListResult{Tools: s.tools.list()},
	}
}

func (s *Server) handleToolsCall(req jsonRPCRequest) *jsonRPCResponse {
	// Parse params as callToolParams.
	data, err := json.Marshal(req.Params)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32602, Message: "Invalid params"},
		}
	}
	var params callToolParams
	if err := json.Unmarshal(data, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	content, err := s.tools.call(params.Name, params.Arguments)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  callToolResult{Content: textContent(err.Error()), IsError: true},
		}
	}
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  callToolResult{Content: content},
	}
}

func (s *Server) handleResourcesList(req jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  resourcesListResult{Resources: s.resources.list()},
	}
}

func (s *Server) handleResourcesRead(req jsonRPCRequest) *jsonRPCResponse {
	data, err := json.Marshal(req.Params)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32602, Message: "Invalid params"},
		}
	}
	var params readResourceParams
	if err := json.Unmarshal(data, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32602, Message: "Invalid params: " + err.Error()},
		}
	}

	content, err := s.resources.read(params.URI)
	if err != nil {
		// Try dynamic plan resources.
		content, dynErr := dynamicPlanResourceRead(s, params.URI)
		if dynErr == nil {
			return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: readResourceResult{Contents: content}}
		}
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32002, Message: err.Error()},
		}
	}
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  readResourceResult{Contents: content},
	}
}
