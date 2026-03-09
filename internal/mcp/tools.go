package mcp

import (
	"encoding/json"
	"fmt"
)

// ToolHandler processes a tool call and returns content blocks.
type ToolHandler func(args map[string]any) ([]contentBlock, error)

type toolRegistry struct {
	definitions []toolDefinition
	handlers    map[string]ToolHandler
}

func newToolRegistry() *toolRegistry {
	return &toolRegistry{
		handlers: make(map[string]ToolHandler),
	}
}

func (r *toolRegistry) register(name, description string, schema map[string]any, handler ToolHandler) {
	r.definitions = append(r.definitions, toolDefinition{
		Name:        name,
		Description: description,
		InputSchema: schema,
	})
	r.handlers[name] = handler
}

func (r *toolRegistry) list() []toolDefinition {
	return r.definitions
}

func (r *toolRegistry) call(name string, args map[string]any) ([]contentBlock, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return handler(args)
}

// Helper to get a string from args.
func argString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Helper to get an int from args.
func argInt(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// Helper to marshal a value to JSON text content.
func jsonContent(value any) ([]contentBlock, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return []contentBlock{{Type: "text", Text: string(data)}}, nil
}

// Helper to return a text content block.
func textContent(text string) []contentBlock {
	return []contentBlock{{Type: "text", Text: text}}
}

// Schema helpers for inputSchema definitions.
func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func intProp(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func boolProp(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

// Helper to get a bool from args.
func argBool(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
