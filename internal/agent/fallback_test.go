package agent

import "testing"

func TestFallbackPolicyIsEmpty(t *testing.T) {
	t.Parallel()
	if !(FallbackPolicy{}).IsEmpty() {
		t.Fatal("zero-value policy should be empty")
	}
	if (FallbackPolicy{OnTransient: Ollama}).IsEmpty() {
		t.Fatal("policy with OnTransient should not be empty")
	}
	if (FallbackPolicy{OnAuth: Ollama}).IsEmpty() {
		t.Fatal("policy with OnAuth should not be empty")
	}
	if (FallbackPolicy{Mappings: map[ID]ID{Claude: Ollama}}).IsEmpty() {
		t.Fatal("policy with mappings should not be empty")
	}
}

func TestFallbackPolicyResolveMapping(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{
		Mappings:    map[ID]ID{Claude: Ollama},
		OnTransient: Gemini,
	}
	fb, ok := policy.Resolve(Claude, ErrorTransient)
	if !ok || fb != Ollama {
		t.Fatalf("expected per-agent mapping to Ollama, got %q ok=%v", fb, ok)
	}
}

func TestFallbackPolicyResolveTransient(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{OnTransient: Ollama}
	fb, ok := policy.Resolve(Claude, ErrorTransient)
	if !ok || fb != Ollama {
		t.Fatalf("expected transient fallback to Ollama, got %q ok=%v", fb, ok)
	}
}

func TestFallbackPolicyResolveAuth(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{OnAuth: Gemini}
	fb, ok := policy.Resolve(Claude, ErrorAuth)
	if !ok || fb != Gemini {
		t.Fatalf("expected auth fallback to Gemini, got %q ok=%v", fb, ok)
	}
}

func TestFallbackPolicyResolveNoMatch(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{OnTransient: Ollama}
	_, ok := policy.Resolve(Claude, ErrorAuth)
	if ok {
		t.Fatal("expected no fallback for auth when only transient configured")
	}
}

func TestFallbackPolicyResolveUnknownClass(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{OnTransient: Ollama, OnAuth: Gemini}
	_, ok := policy.Resolve(Claude, ErrorUnknown)
	if ok {
		t.Fatal("expected no fallback for unknown class")
	}
}

func TestFallbackPolicyResolveRateLimitNoFallback(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{OnTransient: Ollama}
	_, ok := policy.Resolve(Claude, ErrorRateLimit)
	if ok {
		t.Fatal("expected no fallback for rate_limit without mapping")
	}
}

func TestFallbackPolicyResolveEmptyMappingValue(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{
		Mappings:    map[ID]ID{Claude: ""},
		OnTransient: Ollama,
	}
	fb, ok := policy.Resolve(Claude, ErrorTransient)
	if !ok || fb != Ollama {
		t.Fatalf("expected global transient fallback when mapping is empty, got %q ok=%v", fb, ok)
	}
}

func TestFallbackPolicyResolveEmptyPolicy(t *testing.T) {
	t.Parallel()
	policy := FallbackPolicy{}
	_, ok := policy.Resolve(Claude, ErrorTransient)
	if ok {
		t.Fatal("expected no fallback from empty policy")
	}
}
