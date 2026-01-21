package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// CachedResponse represents the data stored in Redis
type CachedResponse struct {
	Result   string `json:"result"`
	CachedAt int64  `json:"cached_at"`
}

func CacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only cache if Redis is available
		if redisClient == nil {
			c.Next()
			return
		}

		// Check for payment headers (Signature/Nonce)
		signature := c.GetHeader("X-402-Signature")
		nonce := c.GetHeader("X-402-Nonce")

		// If no signature, we can't verify payment, so bypass cache
		// (Handler will reject it anyway)
		if signature == "" || nonce == "" {
			c.Next()
			return
		}

		// Read request body to generate cache key
		// Check Content-Length first to reject oversized requests immediately
		const maxBodySize = 10 * 1024 * 1024
		// ContentLength == -1 means unknown (chunked encoding or no header), proceed to MaxBytesReader
		if c.Request.ContentLength > maxBodySize {
			c.Header("Connection", "close")
			c.JSON(413, gin.H{"error": "Payload too large", "max_size": "10MB"})
			c.Abort()
			return
		}

		var requestBody []byte
		var err error
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxBodySize))
			requestBody, err = io.ReadAll(c.Request.Body)
			if err != nil {
				// If body too large, MaxBytesReader returns error
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					c.Header("Connection", "close")
					c.JSON(413, gin.H{"error": "Payload too large", "max_size": "10MB"})
					c.Abort()
					return
				}
				// Other read errors - don't continue to handler since body is corrupted
				log.Printf("[ERROR] Failed to read request body: %v", err)
				c.JSON(500, gin.H{"error": "Failed to read request body"})
				c.Abort()
				return
			}
			// Store body in context for handler reuse
			c.Set("request_body", requestBody)
			// Restore body for any code path (cache hit abort or handler)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Parse body to get text for cache key
		// Note: Cache key is based on text+model at request time. If model env var changes
		// between cache key generation and AI call, there could be a mismatch, but this
		// is acceptable since the model should not change during normal operation.
		var req SummarizeRequest
		if err := json.Unmarshal(requestBody, &req); err != nil {
			// Invalid JSON - reject immediately to prevent cache bypass attacks
			log.Printf("[DEBUG] Invalid JSON in request: %v", err)
			c.JSON(400, gin.H{"error": "Invalid request body", "message": "Request must be valid JSON"})
			c.Abort()
			return
		}

		// Validate text is not empty
		if req.Text == "" {
			c.JSON(400, gin.H{"error": "Invalid request", "message": "text field cannot be empty"})
			c.Abort()
			return
		}

		// Generate Cache Key (include model to prevent cache collisions)
		model := os.Getenv("OPENROUTER_MODEL")
		if model == "" {
			model = "z-ai/glm-4.5-air:free"
		}
		cacheKey := getCacheKey(req.Text, model)

		// Check Cache
		if cached, err := getFromCache(c.Request.Context(), cacheKey); err == nil {
			log.Printf("Cache HIT: %s", cacheKey)

			// Cache HIT! -> Verify Payment *BEFORE* serving
			// verifyPayment creates its own timeout context, so pass request context directly
			verifyResp, paymentCtx, err := verifyPayment(c.Request.Context(), signature, nonce)
			if err != nil {
				log.Printf("Verification error on cache hit: %v", err)
				if errors.Is(err, context.DeadlineExceeded) {
					c.JSON(504, gin.H{"error": "Gateway Timeout", "message": "Verifier request timed out"})
				} else {
					c.JSON(500, gin.H{"error": "Verification Service Failed", "message": "An internal error occurred"})
				}
				c.Abort()
				return
			}

			if !verifyResp.IsValid {
				c.JSON(403, gin.H{"error": "Invalid Signature", "details": verifyResp.Error})
				c.Abort()
				return
			}

			// Payment Verified. Store verification for downstream if needed (though we abort)
			c.Set("payment_verification", verifyResp)
			c.Set("payment_context", paymentCtx)

			// Generate Receipt and Respond
			// We treat the cached result as the AI result
			// Generate receipt for cache hit using current request and cached result.
			// Note: request_hash matches current request, response is from cache,
			// but both are cryptographically valid since cache key ensures identical text.
			if err := generateAndSendReceipt(c, *paymentCtx, verifyResp.RecoveredAddress, requestBody, cached.Result); err != nil {
				log.Printf("Failed to send cached response receipt: %v", err)
				// generateAndSendReceipt already sent an error response (500)
			}
			c.Abort()
			return
		}

		// Cache MISS
		log.Printf("Cache MISS: %s", cacheKey)

		// Prepare to capture response
		writer := &cachedWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			cacheKey:       cacheKey,
		}
		c.Writer = writer

		c.Next()

		// Handler finished. Check status and extract result with proper locking
		writer.mu.RLock()
		statusCode := writer.ResponseWriter.Status()
		bodyBytes := writer.body.Bytes()
		writer.mu.RUnlock()

		if statusCode == 200 {
			// Response format: {"result": "...", "receipt": ...}
			var resp map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &resp); err == nil {
				if result, ok := resp["result"].(string); ok {
					// Store asynchronously with a deadline to prevent indefinite goroutines
					go func(k, v string) {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						storeInCache(ctx, k, v)
					}(cacheKey, result)
				}
			}
		}
	}
}

func getCacheKey(text string, model string) string {
	// IMPORTANT: This cache key ONLY includes text and model.
	// Cache version v1 - if parameters change, increment version to invalidate old caches
	// If callOpenRouter() is modified to accept additional parameters
	// (temperature, max_tokens, top_p, etc.), those MUST be added to
	// this cache key to prevent incorrect cache hits.
	// TODO: Consider accepting a struct with all OpenRouter parameters
	const cacheVersion = "v1"
	combined := cacheVersion + ":" + text + ":" + model
	hash := sha256.Sum256([]byte(combined))
	return "ai:summary:" + hex.EncodeToString(hash[:])
}

func getFromCache(ctx context.Context, key string) (*CachedResponse, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("redis not available")
	}

	val, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var cached CachedResponse
	if err := json.Unmarshal([]byte(val), &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

func storeInCache(ctx context.Context, key string, data string) {
	if redisClient == nil {
		return
	}

	ttl := time.Duration(getEnvAsInt("CACHE_TTL_SECONDS", 3600)) * time.Second

	cached := CachedResponse{
		Result:   data,
		CachedAt: time.Now().Unix(),
	}

	jsonData, err := json.Marshal(cached)
	if err != nil {
		log.Printf("[WARNING] Failed to marshal cache data for key %s: %v", safeKeyPrefix(key), err)
		return
	}

	// Use the context provided by caller (already has 5s timeout from async goroutine)
	if err := redisClient.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		log.Printf("[WARNING] Failed to store in cache for key %s: %v", safeKeyPrefix(key), err)
	}
}

// safeKeyPrefix returns first 32 chars of key for logging, or full key if shorter
func safeKeyPrefix(key string) string {
	if len(key) > 32 {
		return key[:32] + "..."
	}
	return key
}

type cachedWriter struct {
	gin.ResponseWriter
	body     *bytes.Buffer
	cacheKey string
	mu       sync.RWMutex
}

func (w *cachedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *cachedWriter) WriteString(s string) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
