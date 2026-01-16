package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"Simple Text", "Hello World"},
		{"Long Text", strings.Repeat("a", 1000)},
		{"Special Chars", "!@#$%^&*()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := "z-ai/glm-4.5-air:free"
			key1 := getCacheKey(tt.text, model)
			key2 := getCacheKey(tt.text, model)

			// 1. Deterministic
			if key1 != key2 {
				t.Errorf("getCacheKey not deterministic: %s != %s", key1, key2)
			}

			// 2. Format
			if !strings.HasPrefix(key1, "ai:summary:") {
				t.Errorf("Key missing prefix: %s", key1)
			}

			// 3. Length (prefix + 64 hex chars)
			expectedLen := len("ai:summary:") + 64
			if len(key1) != expectedLen {
				t.Errorf("Key length wrong: got %d, want %d", len(key1), expectedLen)
			}
		})
	}
}

func TestCacheKeyUniqueForDifferentInputs(t *testing.T) {
	// Verify that different inputs produce different cache keys
	model := "z-ai/glm-4.5-air:free"
	k1 := getCacheKey("abc", model)
	k2 := getCacheKey("abd", model)
	if k1 == k2 {
		t.Error("Different inputs produced same cache key")
	}
}

// Manual helper to verify SHA logic matches spec
func TestCacheKeySpec(t *testing.T) {
	text := "test"
	model := "z-ai/glm-4.5-air:free"
	const cacheVersion = "v1"
	combined := cacheVersion + ":" + text + ":" + model
	hash := sha256.Sum256([]byte(combined))
	expected := "ai:summary:" + hex.EncodeToString(hash[:])
	actual := getCacheKey(text, model)
	if actual != expected {
		t.Errorf("Spec mismatch: got %s want %s", actual, expected)
	}
}
