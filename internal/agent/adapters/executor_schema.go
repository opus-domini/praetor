package adapters

const executorOutputSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["result", "summary"],
  "properties": {
    "result": {
      "type": "string",
      "enum": ["PASS", "FAIL"]
    },
    "summary": {
      "type": "string",
      "minLength": 1
    },
    "tests": {
      "type": "string"
    },
    "gates": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "tests": {
          "type": "string",
          "enum": ["PASS", "FAIL"]
        },
        "lint": {
          "type": "string",
          "enum": ["PASS", "FAIL"]
        },
        "standards": {
          "type": "string",
          "enum": ["PASS", "FAIL"]
        }
      }
    }
  }
}`

func executorOutputSchema() string {
	return executorOutputSchemaJSON
}
