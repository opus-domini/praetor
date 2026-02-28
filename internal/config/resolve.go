package config

import (
	"errors"
	"fmt"
	"os"
)

// Source indicates where a resolved value came from.
type Source string

const (
	SourceDefault Source = "default"
	SourceGlobal  Source = "config"
	SourceProject Source = "project"
)

// ResolvedValue holds a key's effective value and its source.
type ResolvedValue struct {
	Key    string
	Value  string
	Source Source
	Meta   KeyMeta
}

// ResolveAll computes the effective value and source for every registry key.
func ResolveAll(sections map[string]map[string]string, projectRoot string) []ResolvedValue {
	result := make([]ResolvedValue, len(Registry))
	for i, meta := range Registry {
		result[i] = ResolvedValue{
			Key:    meta.Key,
			Value:  meta.DefaultValue,
			Source: SourceDefault,
			Meta:   meta,
		}
	}

	if global, ok := sections[""]; ok {
		for i, rv := range result {
			if v, exists := global[rv.Key]; exists {
				result[i].Value = v
				result[i].Source = SourceGlobal
			}
		}
	}

	if projectRoot != "" {
		normalizedRoot, err := normalizeProjectPath(projectRoot)
		if err == nil && normalizedRoot != "" {
			for section, values := range sections {
				if section == "" {
					continue
				}
				normalizedSection, normalizeErr := normalizeProjectPath(section)
				if normalizeErr != nil {
					continue
				}
				if normalizedSection != normalizedRoot {
					continue
				}
				for i, rv := range result {
					if v, exists := values[rv.Key]; exists {
						result[i].Value = v
						result[i].Source = SourceProject
					}
				}
				break
			}
		}
	}

	return result
}

// LoadResolved loads the config file and returns resolved values, the config
// file path, and any error. Returns defaults when the file is absent.
func LoadResolved(projectRoot string) ([]ResolvedValue, string, error) {
	path := Path()
	if path == "" {
		return ResolveAll(nil, ""), "", nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ResolveAll(nil, ""), path, nil
	}
	if err != nil {
		return nil, path, fmt.Errorf("load config %s: %w", path, err)
	}

	sections, err := parse(string(data))
	if err != nil {
		return nil, path, fmt.Errorf("load config %s: %w", path, err)
	}

	return ResolveAll(sections, projectRoot), path, nil
}
