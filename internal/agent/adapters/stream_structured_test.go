package adapters

import (
	"encoding/json"
	"testing"
)

func TestParseStreamResultStructuredOutput(t *testing.T) {
	t.Parallel()

	stdout := `{"type":"assistant","message":{"content":[{"type":"text","text":"Analysis complete."}]}}
{"type":"result","result":"Analysis complete.","cost_usd":0.15,"structured_output":{"decision":"PASS","reason":"all good"}}`

	r := parseStreamResult(stdout)
	if r.StructuredOutput == "" {
		t.Fatal("expected structured output to be extracted")
	}
	if r.CostUSD != 0.15 {
		t.Errorf("CostUSD = %v, want %v", r.CostUSD, 0.15)
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(r.StructuredOutput), &parsed); err != nil {
		t.Fatalf("structured output is not valid JSON: %v", err)
	}
	if parsed["decision"] != "PASS" {
		t.Fatalf("decision = %q, want PASS", parsed["decision"])
	}
}

func TestParseStreamResultNoStructuredOutput(t *testing.T) {
	t.Parallel()

	stdout := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}
{"type":"result","result":"hello","cost_usd":0.05}`

	r := parseStreamResult(stdout)
	if r.StructuredOutput != "" {
		t.Fatalf("expected no structured output, got %q", r.StructuredOutput)
	}
	if r.Output != "hello" {
		t.Fatalf("Output = %q, want %q", r.Output, "hello")
	}
}

func TestParseStreamResultStructuredOutputPreservedInOutput(t *testing.T) {
	t.Parallel()

	// Simulate what run() does: prepend structured output to text output
	stdout := `{"type":"assistant","message":{"content":[{"type":"text","text":"I verified everything."}]}}
{"type":"result","result":"I verified everything.","cost_usd":0.10,"structured_output":{"decision":"FAIL","reason":"tests broken","hints":["fix test_foo"]}}`

	r := parseStreamResult(stdout)
	if r.StructuredOutput == "" {
		t.Fatal("expected structured output")
	}
	if r.Output != "I verified everything." {
		t.Fatalf("Output = %q, want %q", r.Output, "I verified everything.")
	}
}

func TestParseStreamResultStructuredOutputInvalidJSON(t *testing.T) {
	t.Parallel()

	stdout := `{"type":"result","result":"hello","cost_usd":0.05,"structured_output":"not json object"}`

	r := parseStreamResult(stdout)
	// "not json object" is valid JSON (a string), so it should be captured
	// But the domain parsers will not match it as a structured output line
	if r.Output != "hello" {
		t.Fatalf("Output = %q, want %q", r.Output, "hello")
	}
}

func TestParseStreamResultStructuredOutputNullSkipped(t *testing.T) {
	t.Parallel()

	stdout := `{"type":"result","result":"hello","cost_usd":0.05,"structured_output":null}`

	r := parseStreamResult(stdout)
	if r.StructuredOutput != "" {
		t.Fatalf("expected empty structured output for null, got %q", r.StructuredOutput)
	}
}

func TestParseStreamResultPreservesBackwardCompatibility(t *testing.T) {
	t.Parallel()

	// parseStreamOutput (the original function) should still work
	stdout := `{"type":"result","result":"done","cost_usd":0.05}`
	output, cost := parseStreamOutput(stdout)
	if output != "done" {
		t.Fatalf("Output = %q, want %q", output, "done")
	}
	if cost != 0.05 {
		t.Errorf("CostUSD = %v, want %v", cost, 0.05)
	}
}

func TestReviewerOutputSchemaIsValidJSON(t *testing.T) {
	t.Parallel()

	schema := reviewerOutputSchema()
	if !json.Valid([]byte(schema)) {
		t.Fatal("reviewer schema is not valid JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		t.Fatalf("failed to parse reviewer schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}
	if _, ok := props["decision"]; !ok {
		t.Fatal("schema missing 'decision' property")
	}
	if _, ok := props["reason"]; !ok {
		t.Fatal("schema missing 'reason' property")
	}
	if _, ok := props["hints"]; !ok {
		t.Fatal("schema missing 'hints' property")
	}

	required, ok := parsed["required"].([]any)
	if !ok {
		t.Fatal("schema missing required array")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	if !requiredSet["decision"] || !requiredSet["reason"] {
		t.Fatalf("schema required fields must include decision and reason, got %v", required)
	}
}

func TestExecutorOutputSchemaIsValidJSON(t *testing.T) {
	t.Parallel()

	schema := executorOutputSchema()
	if !json.Valid([]byte(schema)) {
		t.Fatal("executor schema is not valid JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
		t.Fatalf("failed to parse executor schema: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema missing properties")
	}
	if _, ok := props["result"]; !ok {
		t.Fatal("schema missing 'result' property")
	}
	if _, ok := props["summary"]; !ok {
		t.Fatal("schema missing 'summary' property")
	}
	if _, ok := props["gates"]; !ok {
		t.Fatal("schema missing 'gates' property")
	}

	required, ok := parsed["required"].([]any)
	if !ok {
		t.Fatal("schema missing required array")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	if !requiredSet["result"] || !requiredSet["summary"] {
		t.Fatalf("schema required fields must include result and summary, got %v", required)
	}
}

func TestPlannerOutputSchemaIsValidJSON(t *testing.T) {
	t.Parallel()

	schema := plannerOutputSchema()
	if !json.Valid([]byte(schema)) {
		t.Fatal("planner schema is not valid JSON")
	}
}
