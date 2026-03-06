package config

import "testing"

func TestConfigFromMapParsesMaxParallelTasks(t *testing.T) {
	t.Parallel()

	cfg, err := configFromMap("global", map[string]string{"max-parallel-tasks": "4"})
	if err != nil {
		t.Fatalf("configFromMap returned error: %v", err)
	}
	if cfg.MaxParallelTasks == nil || *cfg.MaxParallelTasks != 4 {
		t.Fatalf("max parallel tasks = %#v, want 4", cfg.MaxParallelTasks)
	}
}

func TestValidateValueRejectsInvalidMaxParallelTasks(t *testing.T) {
	t.Parallel()

	if err := ValidateValue("max-parallel-tasks", "0"); err == nil {
		t.Fatal("expected validation error for zero max parallel tasks")
	}
}
