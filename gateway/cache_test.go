package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestGetCacheKey tests the cache key generation
func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		text1    string
		text2    string
		expected bool // should keys be equal?
	}{
		{
			name:     "Same text produces same key",
			text1:    "Hello World",
			text2:    "Hello World",
			expected: true,
		},
		{
			name:     "Different text produces different keys",
			text1:    "Hello World",
			text2:    "Goodbye World",
			expected: false,
		},
		{
			name:     "Case sensitive",
			text1:    "Hello World",
			text2:    "hello world",
			expected: false,
		},
		{
			name:     "Whitespace matters",
			text1:    "Hello World",
			text2:    "Hello  World",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := getCacheKey(tt.text1)
			key2 := getCacheKey(tt.text2)

			if (key1 == key2) != tt.expected {
				t.Errorf("getCacheKey() got equal=%v, want equal=%v for '%s' vs '%s'",
					key1 == key2, tt.expected, tt.text1, tt.text2)
			}

			// Verify key format
			if len(key1) != len("ai:summary:")+16 {
				t.Errorf("getCacheKey() produced wrong length key: %s (length %d)", key1, len(key1))
			}

			if key1[:11] != "ai:summary:" {
				t.Errorf("getCacheKey() produced wrong prefix: %s", key1)
			}
		})
	}
}

// TestGetCacheKeyDeterministic ensures the same input always produces the same key
func TestGetCacheKeyDeterministic(t *testing.T) {
	text := "This is a test message for deterministic cache key generation"

	keys := make(map[string]int)
	for i := 0; i < 100; i++ {
		key := getCacheKey(text)
		keys[key]++
	}

	if len(keys) != 1 {
		t.Errorf("getCacheKey() is not deterministic, got %d different keys", len(keys))
	}
}

// TestCacheOperations tests storing and retrieving from cache
func TestCacheOperations(t *testing.T) {
	// Skip if Redis is not available
	if redisClient == nil {
		t.Skip("Redis not available, skipping integration test")
	}

	ctx := context.Background()
	testKey := "test:cache:" + time.Now().String()
	testResult := "This is a test summary result"

	// Clean up after test
	defer func() {
		redisClient.Del(ctx, testKey)
	}()

	// Test storing in cache
	storeInCache(testKey, testResult)

	// Give it a moment to complete (it's async)
	time.Sleep(100 * time.Millisecond)

	// Test retrieving from cache
	cached, err := getFromCache(ctx, testKey)
	if err != nil {
		t.Fatalf("getFromCache() failed: %v", err)
	}

	if cached.Result != testResult {
		t.Errorf("getFromCache() got result=%v, want %v", cached.Result, testResult)
	}

	if cached.CachedAt == 0 {
		t.Error("getFromCache() CachedAt is 0")
	}

	// Verify it's actually in Redis
	val, err := redisClient.Get(ctx, testKey).Result()
	if err != nil {
		t.Fatalf("Redis Get failed: %v", err)
	}

	var stored CachedResponse
	if err := json.Unmarshal([]byte(val), &stored); err != nil {
		t.Fatalf("Failed to unmarshal cached data: %v", err)
	}

	if stored.Result != testResult {
		t.Errorf("Stored result mismatch: got %v, want %v", stored.Result, testResult)
	}
}

// TestCacheMiss tests behavior when key doesn't exist
func TestCacheMiss(t *testing.T) {
	if redisClient == nil {
		t.Skip("Redis not available, skipping integration test")
	}

	ctx := context.Background()
	nonExistentKey := "test:nonexistent:" + time.Now().String()

	cached, err := getFromCache(ctx, nonExistentKey)
	if err == nil {
		t.Error("getFromCache() should return error for non-existent key")
	}

	if cached != nil {
		t.Error("getFromCache() should return nil for non-existent key")
	}
}

// TestCacheTTL tests that cache entries expire
func TestCacheTTL(t *testing.T) {
	if redisClient == nil {
		t.Skip("Redis not available, skipping integration test")
	}

	ctx := context.Background()
	testKey := "test:ttl:" + time.Now().String()
	testResult := "This should expire"

	// Store with short TTL
	cached := CachedResponse{
		Result:   testResult,
		CachedAt: time.Now().Unix(),
	}
	jsonData, _ := json.Marshal(cached)
	err := redisClient.Set(ctx, testKey, jsonData, 1*time.Second).Err()
	if err != nil {
		t.Fatalf("Failed to set test key: %v", err)
	}

	// Verify it exists
	_, err = getFromCache(ctx, testKey)
	if err != nil {
		t.Errorf("getFromCache() should succeed before TTL: %v", err)
	}

	// Wait for expiry
	time.Sleep(2 * time.Second)

	// Verify it's gone
	_, err = getFromCache(ctx, testKey)
	if err == nil {
		t.Error("getFromCache() should fail after TTL expiry")
	}
}

// TestCacheEnabledFlag tests the cache enabled check
func TestCacheEnabledFlag(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"Empty string", "", false},
		{"true", "true", true},
		{"1", "1", true},
		{"false", "false", false},
		{"0", "0", false},
		{"TRUE", "TRUE", false}, // Case sensitive
		{"yes", "yes", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			original := redisClient
			defer func() { redisClient = original }()

			// Set env and test
			t.Setenv("CACHE_ENABLED", tt.envValue)
			result := getCacheEnabled()

			if result != tt.expected {
				t.Errorf("getCacheEnabled() with %q = %v, want %v", tt.envValue, result, tt.expected)
			}
		})
	}
}

// TestCacheTTLConfig tests the TTL configuration
func TestCacheTTLConfig(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{"Default", "", 3600 * time.Second},
		{"Custom value", "7200", 7200 * time.Second},
		{"Negative value", "-100", 3600 * time.Second}, // Should default
		{"Zero", "0", 3600 * time.Second},              // Should default
		{"Invalid", "abc", 3600 * time.Second},         // Should default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CACHE_TTL_SECONDS", tt.envValue)
			result := getCacheTTL()

			if result != tt.expected {
				t.Errorf("getCacheTTL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetCacheStats tests cache statistics
func TestGetCacheStats(t *testing.T) {
	ctx := context.Background()
	stats := getCacheStats(ctx)

	if stats == nil {
		t.Fatal("getCacheStats() returned nil")
	}

	// Check required fields
	if _, ok := stats["enabled"]; !ok {
		t.Error("getCacheStats() missing 'enabled' field")
	}

	if _, ok := stats["ttl"]; !ok {
		t.Error("getCacheStats() missing 'ttl' field")
	}

	// If Redis is enabled, check pool stats
	if redisClient != nil {
		requiredFields := []string{"pool_hits", "pool_misses", "pool_timeouts", "total_conns", "idle_conns"}
		for _, field := range requiredFields {
			if _, ok := stats[field]; !ok {
				t.Errorf("getCacheStats() missing '%s' field when Redis is enabled", field)
			}
		}
	}
}

// TestStoreInCacheWithNilClient tests graceful fallback when Redis is unavailable
func TestStoreInCacheWithNilClient(t *testing.T) {
	// Save original client
	original := redisClient
	defer func() { redisClient = original }()

	// Set to nil to simulate Redis unavailable
	redisClient = nil

	// This should not panic
	storeInCache("test:key", "test value")
}

// TestGetFromCacheWithNilClient tests error handling when Redis is unavailable
func TestGetFromCacheWithNilClient(t *testing.T) {
	// Save original client
	original := redisClient
	defer func() { redisClient = original }()

	// Set to nil to simulate Redis unavailable
	redisClient = nil

	ctx := context.Background()

	cached, err := getFromCache(ctx, "test:key")
	if err == nil {
		t.Error("getFromCache() should return error when Redis is nil")
	}

	if cached != nil {
		t.Error("getFromCache() should return nil when Redis is nil")
	}
}

// BenchmarkGetCacheKey benchmarks cache key generation
func BenchmarkGetCacheKey(b *testing.B) {
	text := "This is a sample text that needs to be summarized for benchmarking purposes"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCacheKey(text)
	}
}

// BenchmarkCacheOperations benchmarks cache store and retrieve
func BenchmarkCacheOperations(b *testing.B) {
	if redisClient == nil {
		b.Skip("Redis not available")
	}

	ctx := context.Background()
	text := "This is a sample text for benchmarking"
	result := "This is a sample summary result"
	cacheKey := getCacheKey(text)

	// Clean up
	defer redisClient.Del(ctx, cacheKey)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		storeInCache(cacheKey, result)
		getFromCache(ctx, cacheKey)
	}
}
