package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var defaultAllowedOrigins = []string{"http://localhost:3001"}

const (
	receiptStoreModeRedis  = "redis"
	receiptStoreModeMemory = "memory"
)

func getAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
	if raw == "" {
		return defaultAllowedOrigins
	}

	origins := make([]string, 0)
	for _, entry := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(entry)
		if origin == "" {
			continue
		}
		if isValidAllowedOrigin(origin) {
			origins = append(origins, origin)
		} else {
			log.Printf("Warning: ignoring invalid ALLOWED_ORIGINS entry: %q", origin)
		}
	}
	if len(origins) == 0 {
		return defaultAllowedOrigins
	}

	return origins
}

func isValidAllowedOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func getReceiptStoreMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RECEIPT_STORE")))
	if mode == "" {
		return receiptStoreModeRedis
	}
	return mode
}

func validateReceiptStoreMode() error {
	switch getReceiptStoreMode() {
	case receiptStoreModeRedis, receiptStoreModeMemory:
		return nil
	default:
		return fmt.Errorf("RECEIPT_STORE must be %q or %q", receiptStoreModeRedis, receiptStoreModeMemory)
	}
}

func isRedisRequired() bool {
	return getCacheEnabled() || getReceiptStoreMode() == receiptStoreModeRedis
}

// getPositiveTimeout returns the configured timeout in seconds, but ensures a
// sensible default if the provided value is non-positive.
func getPositiveTimeout(envKey string, defaultSeconds int) time.Duration {
	seconds := getEnvAsInt(envKey, defaultSeconds)
	if seconds <= 0 {
		seconds = defaultSeconds
	}
	return time.Duration(seconds) * time.Second
}

// Timeout helpers (configurable via env vars)
func getRequestTimeout() time.Duration  { return getPositiveTimeout("REQUEST_TIMEOUT_SECONDS", 60) }
func getAITimeout() time.Duration       { return getPositiveTimeout("AI_REQUEST_TIMEOUT_SECONDS", 30) }
func getVerifierTimeout() time.Duration { return getPositiveTimeout("VERIFIER_TIMEOUT_SECONDS", 2) }
func getHealthCheckTimeout() time.Duration {
	return getPositiveTimeout("HEALTH_CHECK_TIMEOUT_SECONDS", 2)
}

// defaultMaxRequestBodyBytes is 10 MiB, the default limit applied when
// MAX_REQUEST_BODY_BYTES is unset or invalid. It matches the previous
// hardcoded constant in handleSummarize and CacheMiddleware.
const defaultMaxRequestBodyBytes int64 = 10 * 1024 * 1024

// getMaxRequestBodySize returns the configured maximum request body size in
// bytes from the MAX_REQUEST_BODY_BYTES environment variable. Returns the
// default (10 MiB) when unset, non-numeric, zero, or negative.
func getMaxRequestBodySize() int64 {
	valStr := os.Getenv("MAX_REQUEST_BODY_BYTES")
	if valStr == "" {
		return defaultMaxRequestBodyBytes
	}
	val, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil || val <= 0 {
		log.Printf("Warning: Invalid MAX_REQUEST_BODY_BYTES %q, using default %d", valStr, defaultMaxRequestBodyBytes)
		return defaultMaxRequestBodyBytes
	}
	return val
}

// formatBytes returns a human-readable byte size string (e.g. "10MB", "512KB")
// used in 413 Payload Too Large error messages so the limit is always accurate.
func formatBytes(b int64) string {
	const (
		mb = 1024 * 1024
		kb = 1024
	)
	switch {
	case b >= mb && b%mb == 0:
		return fmt.Sprintf("%dMB", b/mb)
	case b >= kb && b%kb == 0:
		return fmt.Sprintf("%dKB", b/kb)
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}
