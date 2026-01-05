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
