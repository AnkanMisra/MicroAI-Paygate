package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTimeoutMiddleware_AbortsOnTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestTimeoutMiddleware(1 * time.Second))
	r.GET("/slow", func(c *gin.Context) {
		// Handler intentionally sleeps longer than middleware timeout
		time.Sleep(2 * time.Second)
		c.JSON(200, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/slow", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 504 {
		t.Fatalf("Expected status 504, got %d; body=%s", w.Code, w.Body.String())
	}

	if !strings.Contains(w.Body.String(), "Gateway Timeout") {
		t.Fatalf("Expected Gateway Timeout message, got body: %s", w.Body.String())
	}

	// Ensure handler's response didn't slip through
	if strings.Contains(w.Body.String(), `"ok": true`) {
		t.Fatalf("Handler response should not be present after timeout, got: %s", w.Body.String())
	}
}

func TestCallOpenRouter_RespectsContextTimeout(t *testing.T) {
	// Mock server that responds slowly
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer slow.Close()

	t.Setenv("OPENROUTER_URL", slow.URL)
	t.Setenv("OPENROUTER_API_KEY", "test")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := callOpenRouter(ctx, "hello")
	if err == nil {
		t.Fatalf("Expected timeout error from callOpenRouter, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestHandleSummarize_AIRequestTimeoutReturns504(t *testing.T) {
	// Set up a verifier that returns valid immediately
	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"is_valid":true, "recovered_address":"0xabc","error":""}`))
	}))
	defer verifier.Close()

	// OpenRouter slow server
	slowAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(200)
		w.Write([]byte(`{"choices":[{"message":{"content":"delayed"}}]}`))
	}))
	defer slowAI.Close()

	// Environment
	t.Setenv("OPENROUTER_URL", slowAI.URL)
	t.Setenv("OPENROUTER_API_KEY", "test")
	t.Setenv("VERIFIER_URL", verifier.URL)
	t.Setenv("AI_REQUEST_TIMEOUT_SECONDS", "1")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Apply AI-specific timeout to this route
	r.POST("/api/ai/summarize", RequestTimeoutMiddleware(getAITimeout()), handleSummarize)

	// Build a valid request with signature/nonce
	reqBody := strings.NewReader(`{"text":"hello"}`)
	req, _ := http.NewRequest("POST", "/api/ai/summarize", reqBody)
	req.Header.Set("X-402-Signature", "sig")
	req.Header.Set("X-402-Nonce", "nonce")
	req.Header.Set("X-402-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	start := time.Now()
	r.ServeHTTP(w, req)
	dur := time.Since(start)

	// NOTE: moved fast handler and panic handling into separate tests

	if w.Code != 504 {
		t.Fatalf("Expected 504 Gateway Timeout, got %d; body=%s", w.Code, w.Body.String())
	}

	// Ensure it timed out approximately around 1s (allow more headroom in CI)
	if dur < 900*time.Millisecond || dur > 2*time.Second {
		t.Fatalf("Expected duration around 1s, got %v", dur)
	}

	// Ensure the handler's delayed response didn't make it into the final body
	if strings.Contains(w.Body.String(), "delayed") {
		t.Fatalf("Handler response leaked after timeout: %s", w.Body.String())
	}
}
