package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestCacheIntegration_FullFlow(t *testing.T) {
	// 1. Check Redis availability
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis unavailable, skipping integration test: %v", err)
	}

	// 3. Setup Dependencies (Environment)
	// Mock Verifier
	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock validation based on signature
		var req VerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid verification request", http.StatusBadRequest)
			return
		}

		isValid := req.Signature == "0xValidSig"
		resp := VerifyResponse{
			IsValid:          isValid,
			RecoveredAddress: "0xTestUser",
			Error:            "",
		}
		if !isValid {
			resp.Error = "Invalid signature"
		}

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}))
	defer verifier.Close()

	// Mock OpenRouter (AI)
	// Use small delay to simulate processing so we can verify cache speedup
	var aiCalls atomic.Int32
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"content":"AI Summary Result"}}]}`))
	}))
	defer ai.Close()

	// Set Env Vars using t.Setenv for auto-cleanup
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("REDIS_URL", "127.0.0.1:6379")
	t.Setenv("VERIFIER_URL", verifier.URL)
	t.Setenv("OPENROUTER_URL", ai.URL)
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("RECIPIENT_ADDRESS", "0xTestRecipient")

	// 4. Initialize Gateway logic
	initRedis()
	defer func() {
		if redisClient != nil {
			redisClient.Close()
			redisClient = nil
		}
	}()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestTimeoutMiddleware(5 * time.Second))
	r.POST("/api/ai/summarize", CacheMiddleware(), handleSummarize)

	// 5. Test execution
	textToSummarize := "This is a unique text for cache integration test " + time.Now().String()
	model := "z-ai/glm-4.5-air:free" // Default model
	cacheKey := getCacheKey(textToSummarize, model)

	// Helper to make request
	makeRequest := func(sig string) *httptest.ResponseRecorder {
		t.Helper()
		reqBody := map[string]string{"text": textToSummarize}
		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		req, err := http.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(jsonBody))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-402-Signature", sig)
		req.Header.Set("X-402-Nonce", "nonce-123")
		req.Header.Set("X-402-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Clean up cache key before starting
	rdb.Del(ctx, cacheKey)
	defer rdb.Del(ctx, cacheKey)

	// Request 1: Cache Miss (Valid Sig)
	start := time.Now()
	w1 := makeRequest("0xValidSig")
	duration1 := time.Since(start)

	if w1.Code != 200 {
		t.Fatalf("Request 1 failed: %d body=%s", w1.Code, w1.Body.String())
	}
	if aiCalls.Load() != 1 {
		t.Errorf("Expected 1 AI call, got %d", aiCalls.Load())
	}
	if duration1 < 100*time.Millisecond {
		t.Errorf("Request 1 was too fast (%v), expected >100ms delay", duration1)
	}

	// Wait for async cache set (polling)
	assertCachePopulated := func() {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			exists, err := rdb.Exists(ctx, cacheKey).Result()
			if err == nil && exists > 0 {
				return // Cache populated
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("Cache key %s not populated after 2s", cacheKey)
	}
	assertCachePopulated()

	// Request 2: Cache Hit (Valid Sig)
	start = time.Now()
	w2 := makeRequest("0xValidSig")
	duration2 := time.Since(start)

	if w2.Code != 200 {
		t.Fatalf("Request 2 failed: %d body=%s", w2.Code, w2.Body.String())
	}
	if aiCalls.Load() != 1 {
		t.Errorf("Expected AI calls to stay at 1, got %d (Cache Miss?)", aiCalls.Load())
	}
	// Duration Check (should be significantly faster)
	if duration2 > 50*time.Millisecond {
		t.Logf("Warning: Cache hit was slow (%v), but logic verified.", duration2)
	}

	// Security Check: Cache HIT but INVALID Signature
	w3 := makeRequest("0xInvalidSig")
	if w3.Code != 403 {
		t.Errorf("Expected status 403 for invalid signature on cache hit, got %d", w3.Code)
	}

	// Security Check: Cache HIT but MISSING Signature
	reqBody := map[string]string{"text": textToSummarize}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}
	reqNoSig, err := http.NewRequest("POST", "/api/ai/summarize", bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	reqNoSig.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, reqNoSig)

	if w4.Code != 402 {
		t.Errorf("Expected status 402 for missing signature, got %d", w4.Code)
	}

	// Verify Body
	var resp1, resp2 map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("Failed to unmarshal response 1: %v", err)
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("Failed to unmarshal response 2: %v", err)
	}

	if val, ok := resp1["result"].(string); !ok || val != "AI Summary Result" {
		t.Errorf("Unexpected result 1: %v", resp1["result"])
	}
	if val, ok := resp2["result"].(string); !ok || val != "AI Summary Result" {
		t.Errorf("Unexpected result 2: %v", resp2["result"])
	}
}
