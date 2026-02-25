package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type outputSchemaFile struct {
	schemaPath string
	cleanup    func() error
}

func createOutputSchemaFile(schema any) (outputSchemaFile, error) {
	if schema == nil {
		return outputSchemaFile{
			cleanup: func() error { return nil },
		}, nil
	}

	marshaled, err := json.Marshal(schema)
	if err != nil {
		return outputSchemaFile{}, fmt.Errorf("marshal output schema: %w", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(marshaled, &obj); err != nil || obj == nil {
		return outputSchemaFile{}, errors.New("output schema must be a JSON object")
	}

	schemaDir, err := os.MkdirTemp("", "codex-output-schema-")
	if err != nil {
		return outputSchemaFile{}, fmt.Errorf("create output schema temp dir: %w", err)
	}
	schemaPath := filepath.Join(schemaDir, "schema.json")
	if err := os.WriteFile(schemaPath, marshaled, 0o644); err != nil {
		_ = os.RemoveAll(schemaDir)
		return outputSchemaFile{}, fmt.Errorf("write output schema: %w", err)
	}

	return outputSchemaFile{
		schemaPath: schemaPath,
		cleanup: func() error {
			_ = os.RemoveAll(schemaDir)
			return nil
		},
	}, nil
}
