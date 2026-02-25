package claude

import "testing"

func TestDecodePromptResultMalformedPayloadReturnsError(t *testing.T) {
	t.Parallel()

	_, err := decodePromptResult(SDKMessage{
		Type: "result",
		Raw:  []byte("{invalid-json"),
	})
	if err == nil {
		t.Fatal("expected malformed payload error")
	}
}
