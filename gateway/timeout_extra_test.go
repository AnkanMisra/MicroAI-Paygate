package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTimeoutMiddleware_AllowsFastHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestTimeoutMiddleware(1 * time.Second))
	r.GET("/fast", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	req, _ := http.NewRequest("GET", "/fast", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Expected 200 for fast handler, got %d", w.Code)
	}
}

func TestRequestTimeoutMiddleware_SetsTimeoutContextOnStreamingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestTimeoutMiddleware(1 * time.Second))
	r.POST(summarizeStreamPath, func(c *gin.Context) {
		// Verify the timeout context is present (not a zero/background context)
		if _, ok := c.Request.Context().Deadline(); !ok {
			t.Fatal("Expected streaming path to have a deadline context")
		}
		c.String(200, "chunk")
	})

	req, _ := http.NewRequest("POST", summarizeStreamPath, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("Expected streaming path to pass through, got %d; body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "chunk" {
		t.Fatalf("Expected streaming handler body to pass through, got %q", w.Body.String())
	}
}

func TestRequestTimeoutMiddleware_AbortsStreamingOnGlobalTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestTimeoutMiddleware(50 * time.Millisecond))
	r.POST(summarizeStreamPath, func(c *gin.Context) {
		select {
		case <-c.Request.Context().Done():
			c.String(504, "timeout")
			return
		case <-time.After(5 * time.Second):
			c.String(200, "ok")
		}
	})

	req, _ := http.NewRequest("POST", summarizeStreamPath, nil)
	w := httptest.NewRecorder()
	start := time.Now()
	r.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != 504 {
		t.Fatalf("Expected global timeout to abort streaming handler, got %d; body=%s", w.Code, w.Body.String())
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Expected streaming handler to abort within 500ms, took %v", elapsed)
	}
}

func TestRequestTimeoutMiddleware_PreservesPanicRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default() // Uses Recovery middleware
	r.Use(RequestTimeoutMiddleware(1 * time.Second))
	r.GET("/panic", func(c *gin.Context) { panic("boom") })

	req, _ := http.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Fatalf("Expected 500 from panic + recovery, got %d", w.Code)
	}
}
