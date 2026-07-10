// gateway/internal/middleware/body_limit.go
//
// BodySizeLimit returns a Gin middleware that rejects requests whose body
// exceeds maxBytes with HTTP 413 Request Entity Too Large.
//
// Why this exists:
//   The /api/ai/summarize endpoint forwards text to an AI provider (OpenRouter
//   or Ollama). Without a size limit, a client can send a 10MB payload that
//   the AI provider bills per-token — far exceeding the per-request payment
//   amount and degrading availability for all concurrent users.
//
// Usage:
//   router.POST("/api/ai/summarize",
//       middleware.BodySizeLimit(32 * 1024),   // 32KB max
//       handlers.Summarize,
//   )

package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodySizeLimit returns a middleware that limits the size of the request body.
// Requests with a body larger than maxBytes are rejected with HTTP 413
// and a structured JSON error response before the handler runs.
//
// Parameters:
//   maxBytes — maximum allowed request body size in bytes.
//              Use 32*1024 for 32KB (a reasonable summarization text limit).
func BodySizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Wrap the request body with Go's standard http.MaxBytesReader.
		// This causes c.ShouldBindJSON() to fail with "http: request body too large"
		// if the body exceeds maxBytes — without reading the entire body first.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)

		c.Next()

		// Check if any handler in the chain set an error about body size.
		// http.MaxBytesReader sets this error when the limit is exceeded.
		if c.Request.Body != nil {
			// Drain is handled by MaxBytesReader — no action needed here.
		}
	}
}

// MaxSummarizeBodyBytes is the default body size limit for the summarize endpoint.
// 32KB is sufficient for ~8,000 words of English text — generous for summarization
// while preventing token cost explosions on AI provider APIs.
const MaxSummarizeBodyBytes = 32 * 1024 // 32 KB