package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCorrelationIDMiddleware_GeneratesNewID verifies that a new UUID is generated
// when no X-Correlation-ID header is provided by the client.
func TestCorrelationIDMiddleware_GeneratesNewID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response has X-Correlation-ID header
	correlationID := w.Header().Get("X-Correlation-ID")
	if correlationID == "" {
		t.Error("Expected X-Correlation-ID header to be set, got empty")
	}

	// Verify it's a valid UUID format (8-4-4-4-12)
	parts := strings.Split(correlationID, "-")
	if len(parts) != 5 {
		t.Errorf("Expected UUID format (5 parts), got %d parts: %s", len(parts), correlationID)
	}

	// Verify lengths: 8-4-4-4-12
	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("UUID part %d expected length %d, got %d: %s", i, expectedLengths[i], len(part), correlationID)
		}
	}
}

// TestCorrelationIDMiddleware_PreservesExistingID verifies that when a client
// provides an X-Correlation-ID header, it is preserved and returned.
func TestCorrelationIDMiddleware_PreservesExistingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	clientProvidedID := "client-provided-id-12345"
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", clientProvidedID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response has the same X-Correlation-ID as provided
	responseID := w.Header().Get("X-Correlation-ID")
	if responseID != clientProvidedID {
		t.Errorf("Expected X-Correlation-ID to be '%s', got '%s'", clientProvidedID, responseID)
	}
}

// TestCorrelationIDMiddleware_SetsInGinContext verifies that the correlation ID
// is stored in Gin's context for handler access.
func TestCorrelationIDMiddleware_SetsInGinContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())

	var capturedID string
	r.GET("/test", func(c *gin.Context) {
		// Get from Gin context
		if id, exists := c.Get("correlation_id"); exists {
			capturedID = id.(string)
		}
		c.JSON(200, gin.H{"ok": true})
	})

	clientProvidedID := "gin-context-test-id"
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", clientProvidedID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if capturedID != clientProvidedID {
		t.Errorf("Expected Gin context correlation_id to be '%s', got '%s'", clientProvidedID, capturedID)
	}
}

// TestCorrelationIDMiddleware_SetsInRequestContext verifies that the correlation ID
// is stored in Go's standard context for downstream propagation.
func TestCorrelationIDMiddleware_SetsInRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())

	var capturedID string
	r.GET("/test", func(c *gin.Context) {
		// Get from Go standard context using the typed key
		if id, ok := c.Request.Context().Value(correlationIDKey).(string); ok {
			capturedID = id
		}
		c.JSON(200, gin.H{"ok": true})
	})

	clientProvidedID := "request-context-test-id"
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", clientProvidedID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if capturedID != clientProvidedID {
		t.Errorf("Expected request context correlation_id to be '%s', got '%s'", clientProvidedID, capturedID)
	}
}

// TestCorrelationIDMiddleware_PropagationToDownstream verifies that the correlation ID
// can be retrieved from context for propagation to downstream services.
func TestCorrelationIDMiddleware_PropagationToDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())

	var capturedFromContext string
	r.GET("/test", func(c *gin.Context) {
		// Simulate what verifyPayment and callOpenRouter do
		ctx := c.Request.Context()
		if cid, ok := ctx.Value(correlationIDKey).(string); ok {
			capturedFromContext = cid
		}
		c.JSON(200, gin.H{"ok": true})
	})

	clientProvidedID := "propagation-test-id-abc123"
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Correlation-ID", clientProvidedID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if capturedFromContext != clientProvidedID {
		t.Errorf("Expected context propagation to work with ID '%s', got '%s'", clientProvidedID, capturedFromContext)
	}
}

// TestCorrelationIDMiddleware_UniqueIDsPerRequest verifies that each request
// without a provided ID gets a unique correlation ID.
func TestCorrelationIDMiddleware_UniqueIDsPerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorrelationIDMiddleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		correlationID := w.Header().Get("X-Correlation-ID")
		if correlationID == "" {
			t.Errorf("Request %d: Expected X-Correlation-ID header", i)
			continue
		}

		if ids[correlationID] {
			t.Errorf("Request %d: Duplicate correlation ID detected: %s", i, correlationID)
		}
		ids[correlationID] = true
	}

	if len(ids) != 10 {
		t.Errorf("Expected 10 unique IDs, got %d", len(ids))
	}
}

// TestCorrelationIDKey_TypeSafety verifies that the typed context key prevents
// collisions with string keys.
func TestCorrelationIDKey_TypeSafety(t *testing.T) {
	ctx := context.Background()

	// Set with typed key
	ctx = context.WithValue(ctx, correlationIDKey, "typed-value")

	// Set with plain string key (should not collide)
	ctx = context.WithValue(ctx, "correlation_id", "string-value")

	// Retrieve with typed key - should get typed value
	typedValue, ok := ctx.Value(correlationIDKey).(string)
	if !ok || typedValue != "typed-value" {
		t.Errorf("Expected typed key to return 'typed-value', got '%s'", typedValue)
	}

	// Retrieve with string key - should get string value (different from typed)
	stringValue, ok := ctx.Value("correlation_id").(string)
	if !ok || stringValue != "string-value" {
		t.Errorf("Expected string key to return 'string-value', got '%s'", stringValue)
	}
}
