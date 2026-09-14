package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gateway/internal/ai"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
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
	var verifierMu sync.Mutex
	var verifierRequestHashes []string
	seenNonces := make(map[string]struct{})
	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock validation based on signature
		var req VerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid verification request", http.StatusBadRequest)
			return
		}
		expectedSignature := fmt.Sprintf("0x%x", sha256.Sum256([]byte(req.Context.Nonce+":"+req.Context.RequestHash)))
		verifierMu.Lock()
		_, replayed := seenNonces[req.Context.Nonce]
		isValid := req.Signature == expectedSignature && !replayed
		if isValid {
			seenNonces[req.Context.Nonce] = struct{}{}
		}
		verifierRequestHashes = append(verifierRequestHashes, req.Context.RequestHash)
		verifierMu.Unlock()

		resp := VerifyResponse{
			IsValid:          isValid,
			RecoveredAddress: "0x14791697260e4c9a71f18484c9f997b308e59325",
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
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aiCalls.Add(1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"content":"AI Summary Result"}}]}`))
	}))
	defer aiServer.Close()

	// Set Env Vars using t.Setenv for auto-cleanup
	t.Setenv("CACHE_ENABLED", "true")
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("REDIS_URL", "127.0.0.1:6379")
	t.Setenv("VERIFIER_URL", verifier.URL)
	t.Setenv("AI_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_URL", aiServer.URL)
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("RECIPIENT_ADDRESS", "0xTestRecipient")

	// 4. Initialize Gateway logic
	if err := initRedis(); err != nil {
		t.Fatalf("Failed to initialize Redis: %v", err)
	}
	defer func() {
		if redisClient != nil {
			redisClient.Close()
			redisClient = nil
		}
	}()

	// Initialize AI provider for the test
	var err error
	aiProvider, err = ai.NewProvider()
	if err != nil {
		t.Fatalf("Failed to initialize AI provider: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestTimeoutMiddleware(5 * time.Second))
	r.POST("/api/ai/summarize", CacheMiddleware(), handleSummarize)

	// 5. Test execution
	textToSummarize := "This is a unique text for cache integration test " + time.Now().String()
	compactBody, err := json.Marshal(map[string]string{"text": textToSummarize})
	if err != nil {
		t.Fatalf("Failed to marshal compact request body: %v", err)
	}
	var spacedBody bytes.Buffer
	if err := json.Indent(&spacedBody, compactBody, "", "  "); err != nil {
		t.Fatalf("Failed to format request body: %v", err)
	}
	model := "z-ai/glm-4.5-air:free" // Default model
	cacheKey := getCacheKey(textToSummarize, model)

	// Helper to make a request whose test signature is bound to its nonce and exact body hash.
	makeRequest := func(nonce string, rawBody []byte, validSignature bool) *httptest.ResponseRecorder {
		t.Helper()
		req, err := http.NewRequest("POST", "/api/ai/summarize", bytes.NewReader(rawBody))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		requestHash := fmt.Sprintf("0x%x", sha256.Sum256(rawBody))
		signature := fmt.Sprintf("0x%x", sha256.Sum256([]byte(nonce+":"+requestHash)))
		if !validSignature {
			signature = "0xInvalidSig"
		}
		req.Header.Set("X-402-Signature", signature)
		req.Header.Set("X-402-Payer", "0x14791697260E4c9A71f18484C9f997B308e59325")
		req.Header.Set("X-402-Nonce", nonce)
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
	w1 := makeRequest("nonce-compact", compactBody, true)
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
	w2 := makeRequest("nonce-spaced", spacedBody.Bytes(), true)
	duration2 := time.Since(start)

	if w2.Code != 200 {
		t.Fatalf("Request 2 failed: %d body=%s", w2.Code, w2.Body.String())
	}
	if aiCalls.Load() != 1 {
		t.Errorf("Expected AI calls to stay at 1, got %d (Cache Miss?)", aiCalls.Load())
	}
	verifierMu.Lock()
	gotHashes := append([]string(nil), verifierRequestHashes...)
	verifierMu.Unlock()
	if len(gotHashes) < 2 {
		t.Fatalf("Expected verifier requests for cache miss and hit, got %d", len(gotHashes))
	}
	wantCompactHash := fmt.Sprintf("0x%x", sha256.Sum256(compactBody))
	wantSpacedHash := fmt.Sprintf("0x%x", sha256.Sum256(spacedBody.Bytes()))
	if gotHashes[0] != wantCompactHash || gotHashes[1] != wantSpacedHash {
		t.Fatalf("Verifier request hashes = %v, want [%s %s]", gotHashes[:2], wantCompactHash, wantSpacedHash)
	}
	// Duration Check (should be significantly faster)
	if duration2 > 50*time.Millisecond {
		t.Logf("Warning: Cache hit was slow (%v), but logic verified.", duration2)
	}

	// Security Check: Cache HIT but INVALID Signature
	w3 := makeRequest("nonce-invalid", compactBody, false)
	if w3.Code != 403 {
		t.Errorf("Expected status 403 for invalid signature on cache hit, got %d", w3.Code)
	}

	// Security Check: Cache HIT but MISSING Signature
	reqNoSig, err := http.NewRequest("POST", "/api/ai/summarize", bytes.NewReader(compactBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	reqNoSig.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, reqNoSig)

	if w4.Code != 402 {
		t.Fatalf("Expected status 402 for missing signature, got %d", w4.Code)
	}
	var challenge struct {
		PaymentContext PaymentContext `json:"paymentContext"`
	}
	if err := json.Unmarshal(w4.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("Failed to unmarshal payment challenge: %v", err)
	}
	wantRequestHash := fmt.Sprintf("0x%x", sha256.Sum256(compactBody))
	require.Equal(t, paymentAuthorizationVersion, challenge.PaymentContext.AuthorizationVersion)
	require.Equal(t, "0xTestRecipient", challenge.PaymentContext.Recipient)
	require.Equal(t, "USDC", challenge.PaymentContext.Token)
	require.NotEmpty(t, challenge.PaymentContext.Amount)
	require.NotEmpty(t, challenge.PaymentContext.Nonce)
	require.Positive(t, challenge.PaymentContext.ChainID)
	require.Positive(t, challenge.PaymentContext.Timestamp)
	require.Equal(t, "http://localhost:3000", challenge.PaymentContext.Audience)
	require.Equal(t, http.MethodPost, challenge.PaymentContext.Method)
	require.Equal(t, "/api/ai/summarize", challenge.PaymentContext.Resource)
	require.Equal(t, "application/json", challenge.PaymentContext.ContentType)
	require.Equal(t, wantRequestHash, challenge.PaymentContext.RequestHash)

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
