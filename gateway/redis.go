package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func initRedis() {
	if !getCacheEnabled() {
		return
	}

	// Close existing client if any
	if redisClient != nil {
		redisClient.Close()
	}

	// Parse Redis connection options
	redisURL := getEnv("REDIS_URL", "localhost:6379")
	var opts *redis.Options

	if strings.HasPrefix(redisURL, "redis://") || strings.HasPrefix(redisURL, "rediss://") {
		// Parse full Redis URL
		var err error
		opts, err = redis.ParseURL(redisURL)
		if err != nil {
			log.Printf("WARNING: Invalid REDIS_URL format: %v", err)
			log.Println("Continuing with caching disabled. Set CACHE_ENABLED=false to suppress this warning.")
			redisClient = nil
			return
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
		log.Printf("WARNING: Redis connection failed when CACHE_ENABLED=true: %v", err)
		log.Println("Continuing with caching disabled. Set CACHE_ENABLED=false to suppress this warning.")
		redisClient.Close()
		redisClient = nil
		return
	}
	log.Println("Redis connected successfully")
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
