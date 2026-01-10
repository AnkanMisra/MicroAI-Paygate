package main

import (
	"context"
	"log"
	"os"
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

	redisClient = redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_URL", "localhost:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       getEnvAsInt("REDIS_DB", 0),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("Redis unavailable: %v (caching disabled)", err)
		redisClient.Close()
		redisClient = nil
	} else {
		log.Println("Redis connected successfully")
	}
}

func getCacheEnabled() bool {
	return os.Getenv("CACHE_ENABLED") == "true"
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
