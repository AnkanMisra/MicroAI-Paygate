package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
	// Verify that different users have separate rate limit buckets
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
		req.Header.Set("X-402-Nonce", "user1-11111111") // Different first 8 chars
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
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != 429 {
		t.Error("User 1 should be rate limited")
	}

	// User 2 should still be allowed (different bucket)
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-402-Signature", "sig2")
	req2.Header.Set("X-402-Nonce", "user2-22222222") // Different first 8 chars
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
		{"With both signature and nonce", "sig123", "test-nonce", "nonce:"},
		{"Only nonce (no signature)", "", "test-nonce", "ip:"},
		{"Only signature (no nonce)", "sig123", "", "ip:"},
		{"Neither", "", "", "ip:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.Default()
			r.GET("/test", func(c *gin.Context) {
				key := getRateLimitKey(c)

				if strings.HasPrefix(tt.expectedKey, "nonce:") {
					if !strings.HasPrefix(key, "nonce:") {
						t.Errorf("Expected nonce-based key, got '%s'", key)
					}
					hashPart := strings.TrimPrefix(key, "nonce:")
					if len(hashPart) != 32 {
						t.Errorf("Expected hash to be 32 chars, got %d", len(hashPart))
					}
				} else {
					if !strings.HasPrefix(key, "ip:") {
						t.Errorf("Expected IP-based key, got '%s'", key)
					}
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
func TestHandleReadyz_UnHealthy(t *testing.T) {
	gin.SetMode(gin.TestMode)

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
