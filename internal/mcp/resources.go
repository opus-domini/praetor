package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/config"
	"github.com/opus-domini/praetor/internal/domain"
)

type resourceHandler func() ([]resourceContent, error)

type resourceRegistry struct {
	definitions []resourceDefinition
	handlers    map[string]resourceHandler
}

func newResourceRegistry() *resourceRegistry {
	return &resourceRegistry{
		handlers: make(map[string]resourceHandler),
	}
}

func (r *resourceRegistry) register(uri, name, description, mimeType string, handler resourceHandler) {
	r.definitions = append(r.definitions, resourceDefinition{
		URI:         uri,
		Name:        name,
		Description: description,
		MimeType:    mimeType,
	})
	r.handlers[uri] = handler
}

func (r *resourceRegistry) list() []resourceDefinition {
	return r.definitions
}

func (r *resourceRegistry) read(uri string) ([]resourceContent, error) {
	handler, ok := r.handlers[uri]
	if ok {
		return handler()
	}
	return nil, fmt.Errorf("resource not found: %s", uri)
}

func registerResources(s *Server) {
	s.resources.register(
		"praetor://plans",
		"Plans",
		"List of all plans for the current project",
		"application/json",
		func() ([]resourceContent, error) {
			store, err := resolveStore(s.projectDir, "")
			if err != nil {
				return nil, err
			}
			statuses, err := store.ListPlanStatuses()
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(statuses, "", "  ")
			if err != nil {
				return nil, err
			}
			return []resourceContent{{
				URI:      "praetor://plans",
				MimeType: "application/json",
				Text:     string(data),
			}}, nil
		},
	)

	s.resources.register(
		"praetor://config",
		"Configuration",
		"Resolved configuration for the current project",
		"application/json",
		func() ([]resourceContent, error) {
			resolved, _, err := config.LoadResolved(s.projectDir)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(resolved, "", "  ")
			if err != nil {
				return nil, err
			}
			return []resourceContent{{
				URI:      "praetor://config",
				MimeType: "application/json",
				Text:     string(data),
			}}, nil
		},
	)

	s.resources.register(
		"praetor://agents",
		"Agent Health",
		"Availability status of all AI agent providers",
		"application/json",
		func() ([]resourceContent, error) {
			// Reuse the doctor tool handler.
			handler := s.tools.handlers["doctor"]
			if handler == nil {
				return nil, fmt.Errorf("doctor tool not registered")
			}
			content, err := handler(nil)
			if err != nil {
				return nil, err
			}
			if len(content) == 0 {
				return nil, fmt.Errorf("no agent status available")
			}
			return []resourceContent{{
				URI:      "praetor://agents",
				MimeType: "application/json",
				Text:     content[0].Text,
			}}, nil
		},
	)
}

// dynamicPlanResourceRead handles praetor://plans/{slug} and praetor://plans/{slug}/state URIs.
// This is called by the server when no static resource matches.
func dynamicPlanResourceRead(s *Server, uri string) ([]resourceContent, error) {
	if !strings.HasPrefix(uri, "praetor://plans/") {
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
	path := strings.TrimPrefix(uri, "praetor://plans/")
	parts := strings.SplitN(path, "/", 2)
	slug := parts[0]

	store, err := resolveStore(s.projectDir, "")
	if err != nil {
		return nil, err
	}

	if len(parts) == 1 {
		// praetor://plans/{slug} -> plan JSON
		plan, err := domain.LoadPlan(store.PlanFile(slug))
		if err != nil {
			return nil, err
		}
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return nil, err
		}
		return []resourceContent{{URI: uri, MimeType: "application/json", Text: string(data)}}, nil
	}

	subPath := parts[1]
	switch subPath {
	case "state":
		status, err := store.Status(slug)
		if err != nil {
			return nil, err
		}
		data, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return nil, err
		}
		return []resourceContent{{URI: uri, MimeType: "application/json", Text: string(data)}}, nil
	default:
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
}
