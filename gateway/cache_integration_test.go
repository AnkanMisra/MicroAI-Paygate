package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestCacheMiddlewareIntegration tests the cache middleware with real requests
func TestCacheMiddlewareIntegration(t *testing.T) {
	// Set up test environment
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	// Initialize Redis for testing
	initRedis()
	if redisClient == nil {
		t.Skip("Redis not available for integration test")
	}
	defer closeRedis()

	// Create test router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/ai/summarize", CacheMiddleware(), handleSummarizeTest)

	// Test data
	requestBody := SummarizeRequest{
		Text: "This is a test message for cache integration testing",
	}
	bodyBytes, _ := json.Marshal(requestBody)
	cacheKey := getCacheKey(requestBody.Text)

	// Clean up cache before and after test
	ctx := context.Background()
	defer redisClient.Del(ctx, cacheKey)
	redisClient.Del(ctx, cacheKey)

	t.Run("Cache Miss - First Request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-402-Signature", "0x1234567890abcdef")
		req.Header.Set("X-402-Nonce", "test-nonce-123")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if cached, ok := response["cached"].(bool); ok && cached {
			t.Error("First request should not be from cache")
		}
	})

	// Give async cache store time to complete
	time.Sleep(200 * time.Millisecond)

	t.Run("Cache Hit - Second Request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-402-Signature", "0x1234567890abcdef")
		req.Header.Set("X-402-Nonce", "test-nonce-456")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if cached, ok := response["cached"].(bool); !ok || !cached {
			t.Error("Second request should be from cache")
		}

		if _, ok := response["cache_key"]; !ok {
			t.Error("Cache hit response should include cache_key")
		}

		if _, ok := response["cached_at"]; !ok {
			t.Error("Cache hit response should include cached_at")
		}
	})

	t.Run("No Cache Without Signature", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		// No X-402-Signature header

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should proceed to handler without caching
		if w.Code == 200 {
			var response map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &response)

			if _, ok := response["cached"]; ok {
				t.Error("Request without signature should not use cache")
			}
		}
	})
}

// handleSummarizeTest is a mock handler for testing
func handleSummarizeTest(c *gin.Context) {
	var req SummarizeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	// Mock AI response
	c.JSON(200, gin.H{
		"result": "This is a test summary response for: " + req.Text,
	})
}

// TestCacheMiddlewareWithRedisDown tests graceful fallback when Redis is unavailable
func TestCacheMiddlewareWithRedisDown(t *testing.T) {
	// Save original client
	original := redisClient
	defer func() { redisClient = original }()

	// Simulate Redis being down
	redisClient = nil

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/ai/summarize", CacheMiddleware(), handleSummarizeTest)

	requestBody := SummarizeRequest{
		Text: "Test with Redis down",
	}
	bodyBytes, _ := json.Marshal(requestBody)

	req := httptest.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-402-Signature", "0x1234567890abcdef")
	req.Header.Set("X-402-Nonce", "test-nonce")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should still work, just without caching
	if w.Code != 200 {
		t.Errorf("Expected status 200 even with Redis down, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if _, ok := response["result"]; !ok {
		t.Error("Response should still contain result even with Redis down")
	}

	if cached, ok := response["cached"].(bool); ok && cached {
		t.Error("Should not be cached when Redis is down")
	}
}

// TestCacheKeyConsistency ensures cache keys are consistent across requests
func TestCacheKeyConsistency(t *testing.T) {
	text := "Consistent cache key test"

	key1 := getCacheKey(text)
	key2 := getCacheKey(text)
	key3 := getCacheKey(text)

	if key1 != key2 || key2 != key3 {
		t.Errorf("Cache keys are not consistent: %s, %s, %s", key1, key2, key3)
	}
}

// TestCacheWithDifferentNonces tests that different nonces still hit the same cache
func TestCacheWithDifferentNonces(t *testing.T) {
	t.Setenv("CACHE_ENABLED", "true")

	initRedis()
	if redisClient == nil {
		t.Skip("Redis not available for integration test")
	}
	defer closeRedis()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/ai/summarize", CacheMiddleware(), handleSummarizeTest)

	requestBody := SummarizeRequest{
		Text: "Same text different nonces",
	}
	bodyBytes, _ := json.Marshal(requestBody)
	cacheKey := getCacheKey(requestBody.Text)

	ctx := context.Background()
	defer redisClient.Del(ctx, cacheKey)

	// First request with nonce1
	req1 := httptest.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-402-Signature", "0xabc")
	req1.Header.Set("X-402-Nonce", "nonce-1")

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	time.Sleep(200 * time.Millisecond) // Wait for async cache

	// Second request with different nonce but same text
	req2 := httptest.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-402-Signature", "0xdef")
	req2.Header.Set("X-402-Nonce", "nonce-2")

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	var response2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &response2)

	if cached, ok := response2["cached"].(bool); !ok || !cached {
		t.Error("Same text with different nonce should hit cache")
	}
}

// TestCachePersistenceAcrossRequests verifies cache persists between handler invocations
func TestCachePersistenceAcrossRequests(t *testing.T) {
	if redisClient == nil {
		initRedis()
	}

	if redisClient == nil {
		t.Skip("Redis not available")
	}

	text := "Persistence test text"
	result := "Cached result for persistence"
	cacheKey := getCacheKey(text)
	ctx := context.Background()

	// Store in cache
	storeInCache(ctx, cacheKey, result)
	time.Sleep(100 * time.Millisecond)

	// Retrieve in different context
	cached, err := getFromCache(ctx, cacheKey)
	if err != nil {
		t.Fatalf("Failed to retrieve cached result: %v", err)
	}

	if cached.Result != result {
		t.Errorf("Expected result %q, got %q", result, cached.Result)
	}

	// Clean up
	redisClient.Del(ctx, cacheKey)
}

// TestMain sets up the test environment
func TestMain(m *testing.M) {
	// Set test environment variables
	os.Setenv("CACHE_ENABLED", "true")
	os.Setenv("REDIS_URL", "localhost:6379")
	os.Setenv("CACHE_TTL_SECONDS", "3600")

	// Run tests
	code := m.Run()

	// Cleanup
	if redisClient != nil {
		closeRedis()
	}

	os.Exit(code)
}
