package claude

import (
	"encoding/json"
	"testing"
)

func TestPermissionUpdateJSON(t *testing.T) {
	t.Parallel()

	original := PermissionUpdate{
		Type: "addRules",
		Rules: []PermissionRuleValue{
			{ToolName: "Bash", RuleContent: "allow all bash"},
		},
		Behavior:    PermissionBehaviorAllow,
		Destination: PermissionDestinationProject,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify behavior field is present in JSON.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw["behavior"] != "allow" {
		t.Fatalf("expected behavior=allow in JSON, got %v", raw["behavior"])
	}
	if raw["destination"] != "projectSettings" {
		t.Fatalf("expected destination=projectSettings in JSON, got %v", raw["destination"])
	}

	// Round-trip back to struct.
	var decoded PermissionUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if decoded.Type != original.Type {
		t.Fatalf("type mismatch: got %q, want %q", decoded.Type, original.Type)
	}
	if decoded.Behavior != original.Behavior {
		t.Fatalf("behavior mismatch: got %q, want %q", decoded.Behavior, original.Behavior)
	}
	if decoded.Destination != original.Destination {
		t.Fatalf("destination mismatch: got %q, want %q", decoded.Destination, original.Destination)
	}
	if len(decoded.Rules) != 1 || decoded.Rules[0].ToolName != "Bash" {
		t.Fatalf("rules mismatch: got %+v", decoded.Rules)
	}
}

func TestPermissionUpdateSetModeJSON(t *testing.T) {
	t.Parallel()

	original := PermissionUpdate{
		Type: "setMode",
		Mode: PermissionModeBypassPermissions,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw["mode"] != "bypassPermissions" {
		t.Fatalf("expected mode=bypassPermissions in JSON, got %v", raw["mode"])
	}
	// behavior should be omitted for setMode variant.
	if _, ok := raw["behavior"]; ok {
		t.Fatalf("expected behavior to be omitted for setMode, got %v", raw["behavior"])
	}

	var decoded PermissionUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if decoded.Type != "setMode" {
		t.Fatalf("type mismatch: got %q, want %q", decoded.Type, "setMode")
	}
	if decoded.Mode != PermissionModeBypassPermissions {
		t.Fatalf("mode mismatch: got %q, want %q", decoded.Mode, PermissionModeBypassPermissions)
	}
}

func TestSandboxSettingsJSON(t *testing.T) {
	t.Parallel()

	boolTrue := true
	proxyPort := 8080

	original := SandboxSettings{
		Enabled:                  &boolTrue,
		AutoAllowBashIfSandboxed: &boolTrue,
		Network: &SandboxNetworkConfig{
			AllowedDomains:    []string{"example.com", "api.example.com"},
			AllowLocalBinding: &boolTrue,
			HTTPProxyPort:     &proxyPort,
		},
		Filesystem: &SandboxFilesystemConfig{
			AllowWrite: []string{"/tmp"},
			DenyRead:   []string{"/etc/shadow"},
		},
		ExcludedCommands: []string{"rm"},
		Ripgrep: &SandboxRipgrepConfig{
			Command: "/usr/bin/rg",
			Args:    []string{"--no-heading"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SandboxSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}

	if decoded.Enabled == nil || !*decoded.Enabled {
		t.Fatal("expected enabled=true")
	}
	if decoded.Network == nil {
		t.Fatal("expected network to be non-nil")
	}
	if len(decoded.Network.AllowedDomains) != 2 {
		t.Fatalf("expected 2 allowed domains, got %d", len(decoded.Network.AllowedDomains))
	}
	if decoded.Network.AllowedDomains[0] != "example.com" {
		t.Fatalf("expected first domain example.com, got %q", decoded.Network.AllowedDomains[0])
	}
	if decoded.Network.HTTPProxyPort == nil || *decoded.Network.HTTPProxyPort != 8080 {
		t.Fatalf("expected httpProxyPort=8080, got %v", decoded.Network.HTTPProxyPort)
	}
	if decoded.Filesystem == nil {
		t.Fatal("expected filesystem to be non-nil")
	}
	if len(decoded.Filesystem.AllowWrite) != 1 || decoded.Filesystem.AllowWrite[0] != "/tmp" {
		t.Fatalf("unexpected allowWrite: %v", decoded.Filesystem.AllowWrite)
	}
	if len(decoded.Filesystem.DenyRead) != 1 || decoded.Filesystem.DenyRead[0] != "/etc/shadow" {
		t.Fatalf("unexpected denyRead: %v", decoded.Filesystem.DenyRead)
	}
	if decoded.Ripgrep == nil || decoded.Ripgrep.Command != "/usr/bin/rg" {
		t.Fatalf("unexpected ripgrep config: %+v", decoded.Ripgrep)
	}
	if len(decoded.ExcludedCommands) != 1 || decoded.ExcludedCommands[0] != "rm" {
		t.Fatalf("unexpected excludedCommands: %v", decoded.ExcludedCommands)
	}
}

func TestOutputFormatJSON(t *testing.T) {
	t.Parallel()

	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	original := OutputFormat{
		Type:   "json_schema",
		Schema: schema,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded OutputFormat
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}

	if decoded.Type != "json_schema" {
		t.Fatalf("type mismatch: got %q, want %q", decoded.Type, "json_schema")
	}

	// Verify the schema content survived round-trip.
	var schemaMap map[string]any
	if err := json.Unmarshal(decoded.Schema, &schemaMap); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if schemaMap["type"] != "object" {
		t.Fatalf("expected schema type=object, got %v", schemaMap["type"])
	}
	props, ok := schemaMap["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to be an object, got %T", schemaMap["properties"])
	}
	if _, ok := props["name"]; !ok {
		t.Fatal("expected properties.name to exist")
	}
}
