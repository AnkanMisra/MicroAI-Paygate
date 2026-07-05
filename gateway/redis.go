package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func initRedis() error {
	if !isRedisRequired() {
		if redisClient != nil {
			_ = redisClient.Close()
			redisClient = nil
		}
		return nil
	}

	// Close existing client if any
	if redisClient != nil {
		_ = redisClient.Close()
		redisClient = nil
	}

	// Parse Redis connection options
	redisURL := getEnv("REDIS_URL", "")
	if redisURL == "" {
		return fmt.Errorf("REDIS_URL not set")
	}
	var opts *redis.Options

	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") {
		// Parse full Redis URL
		var err error
		opts, err = redis.ParseURL(redisURL)
		if err != nil {
			return fmt.Errorf("invalid REDIS_URL format: %w", err)
		}
	} else {
		// Treat as host:port and build options manually
		opts = &redis.Options{
			Addr:     redisURL,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       getEnvAsInt("REDIS_DB", 0),
		}
	}

	redisClient = redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		_ = redisClient.Close()
		redisClient = nil
		return fmt.Errorf("redis connection failed: %w", err)
	}
	log.Println("Redis connected successfully")
	return nil
}

func getCacheEnabled() bool {
	enabled := strings.ToLower(os.Getenv("CACHE_ENABLED"))
	return enabled == "true" || enabled == "1"
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

const memoryUsedTxTTL = 30 * 24 * time.Hour

var (
	memoryUsedTx   = make(map[string]time.Time) // signature -> expiry time
	memoryUsedTxMu sync.Mutex
)

// markTransactionUsed checks and records a transaction signature as used atomically.
// It returns true if the transaction was already used, or false if it was successfully marked.
// Signatures are normalized to lowercase before keying to prevent hex-casing bypass attacks.
func markTransactionUsed(ctx context.Context, txHash string) (bool, error) {
	// Normalize to lowercase so the same 65-byte signature in different
	// hex casing maps to the same replay key, preventing casing bypass.
	key := strings.ToLower(txHash)

	if redisClient == nil {
		now := time.Now()
		memoryUsedTxMu.Lock()
		defer memoryUsedTxMu.Unlock()

		// Prune expired entries to prevent unbounded map growth.
		for k, exp := range memoryUsedTx {
			if now.After(exp) {
				delete(memoryUsedTx, k)
			}
		}

		if exp, exists := memoryUsedTx[key]; exists && now.Before(exp) {
			return true, nil
		}
		memoryUsedTx[key] = now.Add(memoryUsedTxTTL)
		return false, nil
	}

	redisKey := "used_tx:" + key
	// SetNX returns true if the key was set, meaning it wasn't used before.
	added, err := redisClient.SetNX(ctx, redisKey, "1", memoryUsedTxTTL).Result()
	if err != nil {
		return false, err
	}
	return !added, nil
}
