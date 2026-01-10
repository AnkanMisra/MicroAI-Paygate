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
		// Limit to 10MB to match handler limit and prevent DoS
		const maxBodySize = 10 * 1024 * 1024
		var requestBody []byte
		var err error
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(maxBodySize))
			requestBody, err = io.ReadAll(c.Request.Body)
			if err != nil {
				// If body too large, MaxBytesReader returns error
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					c.JSON(413, gin.H{"error": "Payload too large", "max_size": "10MB"})
					c.Abort()
					return
				}
				// Other read errors
				c.Next()
				return
			}
			// Restore body
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Parse body to get text
		var req SummarizeRequest
		if err := json.Unmarshal(requestBody, &req); err != nil {
			// Invalid body, let handler handle
			c.Next()
			return
		}

		// Generate Cache Key
		cacheKey := getCacheKey(req.Text)

		// Check Cache
		if cached, err := getFromCache(c.Request.Context(), cacheKey); err == nil {
			log.Printf("Cache HIT: %s...", cacheKey[:16])

			// Cache HIT! -> Verify Payment *BEFORE* serving
			verifyResp, paymentCtx, err := verifyPayment(c.Request.Context(), signature, nonce)
			if err != nil {
				log.Printf("Verification error on cache hit: %v", err)
				if errors.Is(err, context.DeadlineExceeded) {
					c.JSON(504, gin.H{"error": "Gateway Timeout", "message": "Verifier request timed out"})
				} else {
					c.JSON(500, gin.H{"error": "Verification Service Failed", "details": err.Error()})
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
			if err := generateAndSendReceipt(c, *paymentCtx, verifyResp.RecoveredAddress, requestBody, cached.Result); err != nil {
				log.Printf("Failed to send cached response receipt: %v", err)
				// verifyAndSendReceipt handles error response
			}
			c.Abort()
			return
		}

		// Cache MISS
		log.Printf("Cache MISS: %s...", cacheKey[:16])

		// Prepare to capture response
		writer := &cachedWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			cacheKey:       cacheKey,
			ctx:            c.Request.Context(), // Use request context (has timeouts)
		}
		c.Writer = writer

		c.Next()

		// Handler finished. If 200 OK, store in cache.
		// NOTE: writer.Status() might differ if handler hasn't written header yet?
		// But handler should have written 200 via JSON.
		if writer.Status() == 200 {
			// Extract "result" from response body
			// Response format: {"result": "...", "receipt": ...}
			var resp map[string]interface{}
			if err := json.Unmarshal(writer.body.Bytes(), &resp); err == nil {
				if result, ok := resp["result"].(string); ok {
					// Store asynchronously to not block response
					// But use a detached context or background with timeout because
					// request context might be canceled.
					go func(k, v string) {
						storeInCache(context.Background(), k, []byte(v))
					}(cacheKey, result)
				}
			}
		}
	}
}

func getCacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
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

func storeInCache(ctx context.Context, key string, data []byte) {
	if redisClient == nil {
		return
	}

	ttl := time.Duration(getEnvAsInt("CACHE_TTL_SECONDS", 3600)) * time.Second

	cached := CachedResponse{
		Result:   string(data),
		CachedAt: time.Now().Unix(),
	}

	jsonData, err := json.Marshal(cached)
	if err != nil {
		log.Printf("Failed to marshal cache data: %v", err)
		return
	}

	// Create context with timeout for storage
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := redisClient.Set(ctx, key, jsonData, ttl).Err(); err != nil {
		log.Printf("Failed to store in cache: %v", err)
	}
}

type cachedWriter struct {
	gin.ResponseWriter
	body     *bytes.Buffer
	cacheKey string
	ctx      context.Context
}

func (w *cachedWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *cachedWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
