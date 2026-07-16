package main

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
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

func normalizePaygateAudience() error {
	raw := strings.TrimSpace(os.Getenv("PAYGATE_AUDIENCE"))
	if raw == "" {
		return fmt.Errorf("PAYGATE_AUDIENCE is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("PAYGATE_AUDIENCE must be an absolute origin")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("PAYGATE_AUDIENCE scheme must be http or https")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("PAYGATE_AUDIENCE must have a non-empty host")
	}
	if parsed.User != nil {
		return fmt.Errorf("PAYGATE_AUDIENCE must not contain credentials")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("PAYGATE_AUDIENCE must contain an origin only")
	}

	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	canonicalHost := hostname
	if port != "" {
		canonicalHost = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		canonicalHost = "[" + hostname + "]"
	}
	canonical := parsed.Scheme + "://" + canonicalHost
	if err := os.Setenv("PAYGATE_AUDIENCE", canonical); err != nil {
		return fmt.Errorf("failed to normalize PAYGATE_AUDIENCE: %w", err)
	}
	return nil
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
