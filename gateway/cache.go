package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

// CachedResponse represents a cached AI response
type CachedResponse struct {
	Result   string `json:"result"`
	CachedAt int64  `json:"cached_at"`
}

// initRedis initializes the Redis client with health check and graceful fallback
func initRedis() {
	// Check if caching is enabled
	if !getCacheEnabled() {
		log.Println("Cache disabled (CACHE_ENABLED=false)")
		return
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := getEnvAsInt("REDIS_DB", 0)

	redisClient = redis.NewClient(&redis.Options{
		Addr:         redisURL,
		Password:     redisPassword,
		DB:           redisDB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	// Health check with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("Redis unavailable: %v (caching disabled, will fallback gracefully)", err)
		redisClient = nil
		return
	}

	log.Printf("Redis connected successfully at %s", redisURL)
}

// getCacheKey generates a deterministic cache key from content using SHA256
func getCacheKey(text string) string {
	hash := sha256.Sum256([]byte(text))
	// Use first 16 hex chars (64 bits) for key
	return "ai:summary:" + hex.EncodeToString(hash[:])[:16]
}

// getFromCache retrieves a cached response from Redis
func getFromCache(ctx context.Context, key string) (*CachedResponse, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("redis not available")
	}

	// Add timeout to prevent slow Redis operations from blocking requests
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

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

// storeInCache stores a response in Redis with TTL asynchronously
// Cache writes happen in a background goroutine to avoid blocking the response
func storeInCache(_ context.Context, key string, result string) {
	if redisClient == nil {
		return
	}

	// Make copies of key and result for safe use in goroutine
	keyCopy := key
	resultCopy := result
	ttl := getCacheTTL()

	// Execute cache write in background goroutine to avoid blocking
	go func() {
		// Use background context with timeout - independent of original request
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cached := CachedResponse{
			Result:   resultCopy,
			CachedAt: time.Now().Unix(),
		}

		jsonData, err := json.Marshal(cached)
		if err != nil {
			log.Printf("Failed to marshal cache data: %v", err)
			return
		}

		// Store in Redis - this now truly runs in background
		if err := redisClient.Set(ctx, keyCopy, jsonData, ttl).Err(); err != nil {
			log.Printf("Failed to store in cache: %v", err)
		}
	}()
}

// getCacheEnabled checks if caching is enabled via environment variable
func getCacheEnabled() bool {
	enabled := os.Getenv("CACHE_ENABLED")
	return enabled == "true" || enabled == "1"
}

// getCacheTTL returns the cache TTL duration from environment variable
func getCacheTTL() time.Duration {
	seconds := getEnvAsInt("CACHE_TTL_SECONDS", 3600)
	if seconds <= 0 {
		seconds = 3600 // Default to 1 hour
	}
	return time.Duration(seconds) * time.Second
}

// getCacheStats returns basic cache statistics (for future metrics endpoint)
func getCacheStats(ctx context.Context) map[string]interface{} {
	stats := map[string]interface{}{
		"enabled": redisClient != nil,
		"ttl":     getCacheTTL().Seconds(),
	}

	if redisClient != nil {
		poolStats := redisClient.PoolStats()
		stats["pool_hits"] = poolStats.Hits
		stats["pool_misses"] = poolStats.Misses
		stats["pool_timeouts"] = poolStats.Timeouts
		stats["total_conns"] = poolStats.TotalConns
		stats["idle_conns"] = poolStats.IdleConns
	}

	return stats
}

// closeRedis gracefully closes the Redis connection
func closeRedis() {
	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			log.Printf("Error closing Redis connection: %v", err)
		}
	}
}

// Helper function to convert string to int with default
func getEnvAsIntForCache(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		log.Printf("Warning: Invalid value for %s: %s, using default %d", key, valStr, defaultValue)
		return defaultValue
	}
	return val
}
