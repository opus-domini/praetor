package config

import "testing"

func TestRegistryCoversAllAllowedKeys(t *testing.T) {
	t.Parallel()

	for key := range allowedKeys {
		if _, ok := LookupMeta(key); !ok {
			t.Errorf("allowedKeys has %q but Registry does not", key)
		}
	}
}

func TestRegistryHasNoDuplicates(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})
	for _, m := range Registry {
		if _, exists := seen[m.Key]; exists {
			t.Errorf("duplicate registry key %q", m.Key)
		}
		seen[m.Key] = struct{}{}
	}
}

func TestLookupMeta(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()
		meta, ok := LookupMeta("executor")
		if !ok {
			t.Fatal("expected to find executor")
		}
		if meta.DefaultValue != "codex" {
			t.Errorf("expected default codex, got %q", meta.DefaultValue)
		}
		if meta.Category != CategoryAgents {
			t.Errorf("expected category Agents, got %q", meta.Category)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, ok := LookupMeta("nonexistent-key")
		if ok {
			t.Error("expected not found")
		}
	})
}

func TestGroupedByCategoryCoversAll(t *testing.T) {
	t.Parallel()

	groups := GroupedByCategory()
	total := 0
	for _, g := range groups {
		total += len(g.Keys)
	}
	if total != len(Registry) {
		t.Errorf("grouped total %d != registry total %d", total, len(Registry))
	}
}

func TestGroupedByCategoryOrder(t *testing.T) {
	t.Parallel()

	groups := GroupedByCategory()
	if len(groups) != len(CategoryOrder) {
		t.Fatalf("expected %d groups, got %d", len(CategoryOrder), len(groups))
	}
	for i, g := range groups {
		if g.Category != CategoryOrder[i] {
			t.Errorf("group %d: expected %q, got %q", i, CategoryOrder[i], g.Category)
		}
	}
}

func TestIsAllowedKey(t *testing.T) {
	t.Parallel()

	if !IsAllowedKey("executor") {
		t.Error("expected executor to be allowed")
	}
	if IsAllowedKey("nonexistent") {
		t.Error("expected nonexistent to not be allowed")
	}
}

func TestLookupMetaCostPolicyKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key          string
		defaultValue string
		keyType      KeyType
		category     Category
	}{
		{key: "plan-cost-budget-usd", defaultValue: "0", keyType: KeyTypeFloat, category: CategoryLimits},
		{key: "task-cost-budget-usd", defaultValue: "0", keyType: KeyTypeFloat, category: CategoryLimits},
		{key: "cost-budget-warn-threshold", defaultValue: "0.8", keyType: KeyTypeFloat, category: CategoryLimits},
		{key: "cost-budget-enforce", defaultValue: "true", keyType: KeyTypeBool, category: CategoryLimits},
	}

	for _, tt := range tests {
		meta, ok := LookupMeta(tt.key)
		if !ok {
			t.Fatalf("expected to find %q", tt.key)
		}
		if meta.DefaultValue != tt.defaultValue {
			t.Errorf("%s: expected default %q, got %q", tt.key, tt.defaultValue, meta.DefaultValue)
		}
		if meta.Type != tt.keyType {
			t.Errorf("%s: expected type %v, got %v", tt.key, tt.keyType, meta.Type)
		}
		if meta.Category != tt.category {
			t.Errorf("%s: expected category %q, got %q", tt.key, tt.category, meta.Category)
		}
	}
}
