package middleware

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// InitLogger initializes zerolog with level from LOG_LEVEL env var
func InitLogger() {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}

	parsedLevel, err := zerolog.ParseLevel(level)
	if err != nil {
		parsedLevel = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(parsedLevel)

	log.Logger = zerolog.New(os.Stdout).
		With().
		Timestamp().
		Logger()
}

// RequestLogger is a Gin middleware that logs each request in JSON
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		latency := time.Since(start)

		// Optional values set by handlers
		paymentVerified, _ := c.Get("payment_verified")
		userWallet, _ := c.Get("user_wallet")

		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Int64("latency_ms", latency.Milliseconds()).
			Str("client_ip", c.ClientIP()).
			Interface("payment_verified", paymentVerified).
			Interface("user_wallet", userWallet).
			Msg("request completed")
	}
}
