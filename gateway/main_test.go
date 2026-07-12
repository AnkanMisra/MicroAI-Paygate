package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type spyReceiptStore struct {
	getCalls int
	lastID   string
	receipt  *SignedReceipt
	exists   bool
	err      error
}

func (s *spyReceiptStore) Store(ctx context.Context, receipt *SignedReceipt, ttl time.Duration) error {
	return nil
}

func (s *spyReceiptStore) Get(ctx context.Context, id string) (*SignedReceipt, bool, error) {
	s.getCalls++
	s.lastID = id
	return s.receipt, s.exists, s.err
}

func (s *spyReceiptStore) CleanupExpired(ctx context.Context) error {
	return nil
}

func (s *spyReceiptStore) Close() error {
	return nil
}

func TestHandleSummarize_NoHeaders(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/api/ai/summarize", handleSummarize)

	// Request
	req, _ := http.NewRequest("POST", "/api/ai/summarize", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Assertions
	if w.Code != 402 {
		t.Errorf("Expected status 402, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	if response["error"] != "Payment Required" {
		t.Errorf("Expected error 'Payment Required', got '%v'", response["error"])
	}

	if response["paymentContext"] == nil {
		t.Error("Expected paymentContext to be present")
	}
}

func TestGetChainIDDefaultBaseSepolia(t *testing.T) {
	t.Setenv("CHAIN_ID", "")

	if got := getChainID(); got != 84532 {
		t.Fatalf("expected Base Sepolia default chain ID 84532, got %d", got)
	}
}

// Rate Limiting Integration Tests

func TestRateLimitMiddleware_AnonymousUser(t *testing.T) {
	// Setup with rate limiting enabled
	os.Setenv("RATE_LIMIT_ENABLED", "true")
	os.Setenv("RATE_LIMIT_ANONYMOUS_RPM", "60")
	os.Setenv("RATE_LIMIT_ANONYMOUS_BURST", "3")
	defer func() {
		os.Unsetenv("RATE_LIMIT_ENABLED")
		os.Unsetenv("RATE_LIMIT_ANONYMOUS_RPM")
		os.Unsetenv("RATE_LIMIT_ANONYMOUS_BURST")
	}()

	gin.SetMode(gin.TestMode)
	r := gin.Default()

	limiters := initRateLimiters()
	r.Use(RateLimitMiddleware(limiters))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// First 3 requests should succeed (burst)
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Request %d: Expected status 200, got %d", i+1, w.Code)
		}

		// Check rate limit headers
		if w.Header().Get("X-RateLimit-Limit") == "" {
			t.Errorf("Request %d: Missing X-RateLimit-Limit header", i+1)
		}
		if w.Header().Get("X-RateLimit-Remaining") == "" {
			t.Errorf("Request %d: Missing X-RateLimit-Remaining header", i+1)
		}
		if w.Header().Get("X-RateLimit-Reset") == "" {
			t.Errorf("Request %d: Missing X-RateLimit-Reset header", i+1)
		}
	}

	// 4th request should be rate limited
	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 429 {
		t.Errorf("Expected status 429, got %d", w.Code)
	}

	// Check 429 response headers
	if w.Header().Get("Retry-After") == "" {
		t.Error("Missing Retry-After header in 429 response")
	}
	if w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("Expected X-RateLimit-Remaining to be 0, got %s", w.Header().Get("X-RateLimit-Remaining"))
	}

	// Check 429 response body
	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["error"] != "Too Many Requests" {
		t.Errorf("Expected error 'Too Many Requests', got '%v'", response["error"])
	}
}

func TestRateLimitMiddleware_StandardUser(t *testing.T) {
	// Setup with higher limits for authenticated users
	os.Setenv("RATE_LIMIT_ENABLED", "true")
	os.Setenv("RATE_LIMIT_STANDARD_RPM", "120")
	os.Setenv("RATE_LIMIT_STANDARD_BURST", "5")
	defer func() {
		os.Unsetenv("RATE_LIMIT_ENABLED")
		os.Unsetenv("RATE_LIMIT_STANDARD_RPM")
		os.Unsetenv("RATE_LIMIT_STANDARD_BURST")
	}()

	gin.SetMode(gin.TestMode)
	r := gin.Default()

	limiters := initRateLimiters()
	r.Use(RateLimitMiddleware(limiters))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Authenticated request with signature and nonce
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-402-Signature", "0x1234567890abcdef")
		req.Header.Set("X-402-Nonce", "test-nonce-123")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Request %d: Expected status 200, got %d", i+1, w.Code)
		}

		// Verify rate limit is higher for authenticated users
		limit, _ := strconv.Atoi(w.Header().Get("X-RateLimit-Limit"))
		if limit != 120 {
			t.Errorf("Expected rate limit of 120 for authenticated user, got %d", limit)
		}
	}
}

func TestRateLimitMiddleware_DifferentKeys(t *testing.T) {
	// Verify that different IPs have separate rate limit buckets
	os.Setenv("RATE_LIMIT_ENABLED", "true")
	os.Setenv("RATE_LIMIT_STANDARD_RPM", "60")
	os.Setenv("RATE_LIMIT_STANDARD_BURST", "2")
	defer func() {
		os.Unsetenv("RATE_LIMIT_ENABLED")
		os.Unsetenv("RATE_LIMIT_STANDARD_RPM")
		os.Unsetenv("RATE_LIMIT_STANDARD_BURST")
	}()

	gin.SetMode(gin.TestMode)
	r := gin.Default()

	limiters := initRateLimiters()
	r.Use(RateLimitMiddleware(limiters))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// User 1 exhausts their limit
	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		req.Header.Set("X-402-Signature", "sig1")
		req.Header.Set("X-402-Nonce", "user1-11111111")
		req.RemoteAddr = "192.168.1.1:12345" // Explicit IP for User 1
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Errorf("User 1 request %d should succeed", i+1)
		}
	}

	// User 1 should now be rate limited
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-402-Signature", "sig1")
	req1.Header.Set("X-402-Nonce", "user1-11111111")
	req1.RemoteAddr = "192.168.1.1:12345" // Same IP as User 1
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != 429 {
		t.Error("User 1 should be rate limited")
	}

	// User 2 should still be allowed (different bucket)
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-402-Signature", "sig2")
	req2.Header.Set("X-402-Nonce", "user2-22222222")
	req2.RemoteAddr = "192.168.1.2:12345" // Different IP for User 2
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Error("User 2 should not be rate limited (separate bucket)")
	}
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	// Test that rate limiting can be disabled
	os.Setenv("RATE_LIMIT_ENABLED", "false")
	defer os.Unsetenv("RATE_LIMIT_ENABLED")

	gin.SetMode(gin.TestMode)
	r := gin.Default()

	// Should not apply middleware when disabled
	if getRateLimitEnabled() {
		limiters := initRateLimiters()
		r.Use(RateLimitMiddleware(limiters))
	}

	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Make many requests - all should succeed
	for i := 0; i < 20; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Errorf("Request %d: Expected status 200 (rate limiting disabled), got %d", i+1, w.Code)
		}
	}
}

func TestGetRateLimitKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		signature   string
		nonce       string
		expectedKey string
	}{
		{"IP-based rate limiting (with signature and nonce)", "sig123", "test-nonce", "ip:"},
		{"IP-based rate limiting (with nonce only)", "", "test-nonce", "ip:"},
		{"IP-based rate limiting (with signature only)", "sig123", "", "ip:"},
		{"IP-based rate limiting (without auth headers)", "", "", "ip:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.Default()
			r.GET("/test", func(c *gin.Context) {
				key := getRateLimitKey(c)

				// After fix: All keys should be IP-based to prevent
				// infinite bucket attacks from unique nonces
				if !strings.HasPrefix(key, "ip:") {
					t.Errorf("Expected IP-based key, got '%s'", key)
				}
				c.JSON(200, gin.H{"key": key})
			})

			req, _ := http.NewRequest("GET", "/test", nil)
			if tt.signature != "" {
				req.Header.Set("X-402-Signature", tt.signature)
			}
			if tt.nonce != "" {
				req.Header.Set("X-402-Nonce", tt.nonce)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		})
	}
}

func TestSelectRateLimitTier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		signature    string
		nonce        string
		expectedTier string
	}{
		{"Anonymous (no headers)", "", "", "anonymous"},
		{"Anonymous (only signature)", "sig", "", "anonymous"},
		{"Anonymous (only nonce)", "", "nonce", "anonymous"},
		{"Standard (both headers)", "sig", "nonce", "standard"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.Default()
			r.GET("/test", func(c *gin.Context) {
				tier := selectRateLimitTier(c)
				if tier != tt.expectedTier {
					t.Errorf("Expected tier '%s', got '%s'", tt.expectedTier, tier)
				}
				c.JSON(200, gin.H{"tier": tier})
			})

			req, _ := http.NewRequest("GET", "/test", nil)
			if tt.signature != "" {
				req.Header.Set("X-402-Signature", tt.signature)
			}
			if tt.nonce != "" {
				req.Header.Set("X-402-Nonce", tt.nonce)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		})
	}
}

func TestRateLimitMiddleware_HeadersInResponse(t *testing.T) {
	os.Setenv("RATE_LIMIT_ENABLED", "true")
	os.Setenv("RATE_LIMIT_ANONYMOUS_BURST", "10")
	defer os.Unsetenv("RATE_LIMIT_ENABLED")

	gin.SetMode(gin.TestMode)
	r := gin.Default()

	limiters := initRateLimiters()
	r.Use(RateLimitMiddleware(limiters))
	r.POST("/api/ai/summarize", handleSummarize)

	// Make a request that returns 402 (no auth)
	reqBody := bytes.NewBufferString(`{"text":"test"}`)
	req, _ := http.NewRequest("POST", "/api/ai/summarize", reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Even 402 responses should have rate limit headers
	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("Missing X-RateLimit-Limit header")
	}
	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("Missing X-RateLimit-Remaining header")
	}
	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("Missing X-RateLimit-Reset header")
	}
}
func TestHandleHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.Default()
	r.GET("/healthz", handleHealthz)

	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	require.Equal(t, "ok", response["status"])
	require.Equal(t, "gateway", response["service"])
	require.NotNil(t, response["timestamp"])
}

func TestHandleReadyz_Healthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("CACHE_ENABLED", "false")

	// save originals
	origVerifier := checkVerifierHealth
	origOpenRouter := checkOpenRouterHealth
	defer func() {
		checkVerifierHealth = origVerifier
		checkOpenRouterHealth = origOpenRouter
	}()

	// stub healthy
	checkVerifierHealth = func() string { return "ok" }
	checkOpenRouterHealth = func() string { return "ok" }

	r := gin.Default()
	r.GET("/readyz", handleReadyz)

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	require.Equal(t, true, response["ready"])
	require.NotNil(t, response["timestamp"])

	checks := response["checks"].(map[string]interface{})
	require.Equal(t, "ok", checks["verifier"])
	require.Equal(t, "ok", checks["openrouter"])

	gatewayChecks := checks["gateway"].(map[string]interface{})
	require.Equal(t, "ok", gatewayChecks["status"])
	require.NotZero(t, gatewayChecks["goroutines"])
	require.Contains(t, gatewayChecks, "memory_alloc_mb")
	require.Contains(t, gatewayChecks, "memory_sys_mb")
}

func TestHandleReadyz_OllamaProviderUsesOllamaHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("AI_PROVIDER", "ollama")
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("CACHE_ENABLED", "false")

	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer ollamaServer.Close()
	t.Setenv("OLLAMA_URL", ollamaServer.URL)

	origVerifier := checkVerifierHealth
	origOpenRouter := checkOpenRouterHealth
	defer func() {
		checkVerifierHealth = origVerifier
		checkOpenRouterHealth = origOpenRouter
	}()

	checkVerifierHealth = func() string { return "ok" }
	checkOpenRouterHealth = func() string { return "unconfigured" }

	r := gin.Default()
	r.GET("/readyz", handleReadyz)

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, true, response["ready"])

	checks := response["checks"].(map[string]interface{})
	require.Equal(t, "ok", checks["ollama"])
	require.NotContains(t, checks, "openrouter")

	aiProvider := checks["ai_provider"].(map[string]interface{})
	require.Equal(t, "ollama", aiProvider["provider"])
	require.Equal(t, "ok", aiProvider["status"])
}

func TestCheckOpenRouterHealthAcceptsChatCompletionsURL(t *testing.T) {
	openRouterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer openRouterServer.Close()

	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_URL", openRouterServer.URL+"/api/v1/chat/completions")

	require.Equal(t, "ok", checkOpenRouterHealth())
}

func TestHandleReadyz_UnHealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("CACHE_ENABLED", "false")

	origVerifier := checkVerifierHealth
	origOpenRouter := checkOpenRouterHealth
	defer func() {
		checkVerifierHealth = origVerifier
		checkOpenRouterHealth = origOpenRouter
	}()

	// one dependency unhealthy
	checkVerifierHealth = func() string { return "unreachable" }
	checkOpenRouterHealth = func() string { return "ok" }

	r := gin.Default()
	r.GET("/readyz", handleReadyz)

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	require.Equal(t, false, response["ready"])
	require.NotNil(t, response["timestamp"])

	checks := response["checks"].(map[string]interface{})
	require.Equal(t, "unreachable", checks["verifier"])
}

func TestHandleReadyz_RedisDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("RECEIPT_STORE", "memory")

	// Save originals
	origVerifier := checkVerifierHealth
	origOpenRouter := checkOpenRouterHealth
	origCacheEnabled, cacheWasSet := os.LookupEnv("CACHE_ENABLED")
	defer func() {
		checkVerifierHealth = origVerifier
		checkOpenRouterHealth = origOpenRouter
		if cacheWasSet {
			os.Setenv("CACHE_ENABLED", origCacheEnabled)
		} else {
			os.Unsetenv("CACHE_ENABLED")
		}
	}()

	// Stub healthy services
	checkVerifierHealth = func() string { return "ok" }
	checkOpenRouterHealth = func() string { return "ok" }
	os.Setenv("CACHE_ENABLED", "false")

	r := gin.Default()
	r.GET("/readyz", handleReadyz)

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	require.Equal(t, true, response["ready"])
	checks := response["checks"].(map[string]interface{})
	require.Equal(t, "disabled", checks["redis"])
}

func TestHandleReadyz_RedisUnreachable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Save originals
	origVerifier := checkVerifierHealth
	origOpenRouter := checkOpenRouterHealth
	origCacheEnabled, cacheWasSet := os.LookupEnv("CACHE_ENABLED")
	origRedisClient := redisClient
	defer func() {
		checkVerifierHealth = origVerifier
		checkOpenRouterHealth = origOpenRouter
		if cacheWasSet {
			os.Setenv("CACHE_ENABLED", origCacheEnabled)
		} else {
			os.Unsetenv("CACHE_ENABLED")
		}
		redisClient = origRedisClient
	}()

	// Stub healthy services but Redis is nil (unreachable)
	checkVerifierHealth = func() string { return "ok" }
	checkOpenRouterHealth = func() string { return "ok" }
	os.Setenv("CACHE_ENABLED", "true")
	redisClient = nil // Simulate Redis not initialized

	r := gin.Default()
	r.GET("/readyz", handleReadyz)

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	require.Equal(t, false, response["ready"])
	checks := response["checks"].(map[string]interface{})
	require.Equal(t, "unreachable", checks["redis"])
}
func TestIsValidReceiptID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"valid", "rcpt_abc123def456", true},
		{"empty", "", false},
		{"prefix only", "rcpt_", false},
		{"short", "rcpt_abc", false},
		{"long", "rcpt_abc123def456789", false},
		{"uppercase", "rcpt_ABC123DEF456", false},
		{"non hex", "rcpt_zzzzzzzzzzzz", false},
		{"wrong prefix", "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidReceiptID(tt.id); got != tt.want {
				t.Fatalf("isValidReceiptID(%q) = %v, want %v",
					tt.id, got, tt.want)
			}
		})
	}
}

func TestHandleGetReceipt_ValidationAndLookupPaths(t *testing.T) {
	tests := []struct {
		name             string
		useRouter        bool
		id               string
		expectedStatus   int
		expectedBody     map[string]string
		expectedGetCalls int
	}{
		{
			name:           "empty id",
			id:             "",
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]string{
				"error":   "invalid receipt id format",
				"message": "receipt id must start with rcpt_ followed by exactly 12 lowercase hexadecimal characters",
			},
			expectedGetCalls: 0,
		},
		{
			name:           "prefix only",
			id:             "rcpt_",
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]string{
				"error":   "invalid receipt id format",
				"message": "receipt id must start with rcpt_ followed by exactly 12 lowercase hexadecimal characters",
			},
			expectedGetCalls: 0,
		},
		{
			name:           "malformed id",
			useRouter:      true,
			id:             "foo",
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]string{
				"error":   "invalid receipt id format",
				"message": "receipt id must start with rcpt_ followed by exactly 12 lowercase hexadecimal characters",
			},
			expectedGetCalls: 0,
		},
		{
			name:           "well formed but missing",
			useRouter:      true,
			id:             "rcpt_a1b2c3d4e5f6",
			expectedStatus: http.StatusNotFound,
			expectedBody: map[string]string{
				"error":   "Receipt not found",
				"message": "Receipt may have expired or never existed",
			},
			expectedGetCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &spyReceiptStore{}

			oldStore := getActiveReceiptStore()
			setActiveReceiptStore(spy)
			defer setActiveReceiptStore(oldStore)

			w := httptest.NewRecorder()

			if tt.useRouter {
				router := gin.New()
				router.GET("/api/receipts/:id", handleGetReceipt)
				req := httptest.NewRequest(http.MethodGet, "/api/receipts/"+tt.id, nil)
				router.ServeHTTP(w, req)
			} else {
				c, _ := gin.CreateTestContext(w)
				c.Params = gin.Params{gin.Param{Key: "id", Value: tt.id}}
				handleGetReceipt(c)
			}

			require.Equal(t, tt.expectedStatus, w.Code)
			require.Equal(t, tt.expectedGetCalls, spy.getCalls)

			var body map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tt.expectedBody, body)
		})
	}
}
