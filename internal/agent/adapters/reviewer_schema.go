package adapters

const reviewerOutputSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["decision", "reason"],
  "properties": {
    "decision": {
      "type": "string",
      "enum": ["PASS", "FAIL"]
    },
    "reason": {
      "type": "string",
      "minLength": 1
    },
    "hints": {
      "type": "array",
      "items": {
        "type": "string",
        "minLength": 1
      }
    }
  }
}`

func reviewerOutputSchema() string {
	return reviewerOutputSchemaJSON
}
