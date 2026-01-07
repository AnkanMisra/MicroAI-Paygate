package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
)

// CacheMiddleware wraps responses to store them in cache AFTER payment verification
// SECURITY: Does NOT serve cached responses - cache lookup happens in handler after verification
func CacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if caching is disabled
		if redisClient == nil {
			c.Next()
			return
		}

		// Only cache requests with payment signature
		signature := c.GetHeader("X-402-Signature")
		if signature == "" {
			c.Next()
			return
		}

		// Read and restore request body for cache key generation
		var bodyBytes []byte
		if c.Request.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(c.Request.Body)
			if err != nil {
				log.Printf("CacheMiddleware: failed to read body: %v", err)
				c.Next()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Extract request to generate cache key
		var req SummarizeRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			// Can't parse request, skip caching
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Next()
			return
		}

		// Restore body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Generate and store cache key for handler to use
		cacheKey := getCacheKey(req.Text)
		c.Set("cache_key", cacheKey)
		c.Set("cache_request", req) // Pass parsed request to avoid re-parsing

		// Wrap the response writer to capture responses for caching
		writer := &cacheResponseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			cacheKey:       cacheKey,
			ctx:            c.Request.Context(),
			ginCtx:         c,
			mu:             &sync.Mutex{},
		}
		c.Writer = writer

		c.Next()

		// After handler completes, store in cache if successful
		writer.storeIfSuccess()
	}
}

// cacheResponseWriter wraps gin.ResponseWriter to capture responses for caching
type cacheResponseWriter struct {
	gin.ResponseWriter
	body     *bytes.Buffer
	cacheKey string
	ctx      context.Context
	ginCtx   *gin.Context
	mu       *sync.Mutex
	stored   bool
}

// Write captures the response body while also writing to the underlying writer
func (w *cacheResponseWriter) Write(data []byte) (int, error) {
	// Write to underlying writer first (don't hold lock during I/O)
	n, err := w.ResponseWriter.Write(data)
	// Only lock for the buffer write
	w.mu.Lock()
	w.body.Write(data[:n]) // Capture what was actually written
	w.mu.Unlock()
	return n, err
}

// WriteString captures string responses
func (w *cacheResponseWriter) WriteString(s string) (int, error) {
	// Write to underlying writer first (don't hold lock during I/O)
	n, err := w.ResponseWriter.WriteString(s)
	// Only lock for the buffer write
	w.mu.Lock()
	w.body.WriteString(s[:n]) // Capture what was actually written
	w.mu.Unlock()
	return n, err
}

// storeIfSuccess stores the response in cache if it was a 200 OK
func (w *cacheResponseWriter) storeIfSuccess() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stored {
		return
	}

	// Only cache successful responses (200 OK)
	if w.Status() != 200 {
		return
	}

	// Don't re-cache responses that were served from cache
	if fromCache, exists := w.ginCtx.Get("from_cache"); exists && fromCache.(bool) {
		return
	}

	// Parse the response to extract the result
	var response map[string]interface{}
	if err := json.Unmarshal(w.body.Bytes(), &response); err != nil {
		log.Printf("Failed to parse response for caching: %v", err)
		return
	}

	result, ok := response["result"].(string)
	if !ok {
		log.Printf("Response does not contain 'result' field, skipping cache")
		return
	}

	// Store in cache (storeInCache is already async)
	storeInCache(w.cacheKey, result)
	log.Printf("Cache STORE initiated: %s", w.cacheKey[:16])
	w.stored = true
}
