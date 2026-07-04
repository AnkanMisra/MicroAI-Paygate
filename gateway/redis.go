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

var (
	memoryUsedTx   = make(map[string]bool)
	memoryUsedTxMu sync.Mutex
)

// markTransactionUsed checks and records a transaction signature as used atomically.
// It returns true if the transaction was already used, or false if it was successfully marked.
func markTransactionUsed(ctx context.Context, txHash string) (bool, error) {
	if redisClient == nil {
		memoryUsedTxMu.Lock()
		defer memoryUsedTxMu.Unlock()
		if memoryUsedTx[txHash] {
			return true, nil
		}
		memoryUsedTx[txHash] = true
		return false, nil
	}
	key := "used_tx:" + txHash
	// SetNX returns true if the key was set, meaning it wasn't used before.
	added, err := redisClient.SetNX(ctx, key, "1", 30*24*time.Hour).Result()
	if err != nil {
		return false, err
	}
	return !added, nil
}
