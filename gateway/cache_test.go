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
			key1 := getCacheKey(tt.text)
			key2 := getCacheKey(tt.text)

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

func TestCacheKeyCollisionWait(t *testing.T) {
	// Not a real collision test, but ensuring different content = different key
	k1 := getCacheKey("abc")
	k2 := getCacheKey("abd")
	if k1 == k2 {
		t.Error("Collision detected for different content")
	}
}

// Manual helper to verify SHA logic matches spec
func TestCacheKeySpec(t *testing.T) {
	text := "test"
	hash := sha256.Sum256([]byte(text))
	expected := "ai:summary:" + hex.EncodeToString(hash[:])
	actual := getCacheKey(text)
	if actual != expected {
		t.Errorf("Spec mismatch: got %s want %s", actual, expected)
	}
}
