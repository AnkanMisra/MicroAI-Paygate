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

// CacheMiddleware implements cache-aside pattern for AI responses
// SECURITY: Cache is checked AFTER payment verification in the handler
// This middleware only wraps the response writer to store successful responses
func CacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if caching is disabled
		if redisClient == nil {
			c.Next()
			return
		}

		// Read and restore request body for cache key generation
		var bodyBytes []byte
		if c.Request.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(c.Request.Body)
			if err != nil {
				log.Printf("CacheMiddleware: failed to read request body: %v", err)
				c.Next()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Extract request to generate cache key
		var req SummarizeRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			// If we can't parse the request, just skip caching
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			c.Next()
			return
		}

		// Restore body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Generate cache key
		cacheKey := getCacheKey(req.Text)

	// Try to get from cache
	if cached, err := getFromCache(c.Request.Context(), cacheKey); err == nil {
		log.Printf("Cache HIT: %s (saved API call)", cacheKey[:16])
		// Mark this as cached so we don't re-cache it
		c.Set("from_cache", true)
		c.JSON(200, gin.H{
			"result":    cached.Result,
			"cached":    true,
			"cached_at": cached.CachedAt,
			"cache_key": cacheKey[:16],
		})
		c.Abort()
		return
	}

	log.Printf("Cache MISS: %s (will call API)", cacheKey[:16])
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
	w.mu.Lock()
	if _, err := w.body.Write(data); err != nil {
		log.Printf("Failed to buffer response for caching: %v", err)
	}
	w.mu.Unlock()
	return w.ResponseWriter.Write(data)
}

// WriteString captures string responses
func (w *cacheResponseWriter) WriteString(s string) (int, error) {
	w.mu.Lock()
	if _, err := w.body.WriteString(s); err != nil {
		log.Printf("Failed to buffer response string for caching: %v", err)
	}
	w.mu.Unlock()
	return w.ResponseWriter.WriteString(s)
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

	// Store in cache (async, don't block)
	go storeInCache(w.ctx, w.cacheKey, result)
	log.Printf("Cache STORE: %s", w.cacheKey[:16])
	w.stored = true
}
