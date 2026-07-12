package main

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestInitRedisClosesClientWhenRedisNotRequired(t *testing.T) {
	originalClient := redisClient
	staleClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	redisClient = staleClient
	t.Cleanup(func() {
		if redisClient != nil && redisClient != originalClient {
			_ = redisClient.Close()
		}
		redisClient = originalClient
	})

	t.Setenv("RECEIPT_STORE", "memory")
	t.Setenv("CACHE_ENABLED", "false")

	if err := initRedis(); err != nil {
		t.Fatalf("initRedis returned unexpected error: %v", err)
	}
	if redisClient != nil {
		t.Fatalf("redisClient should be cleared when Redis is not required")
	}
	if err := staleClient.Ping(context.Background()).Err(); !errors.Is(err, redis.ErrClosed) {
		t.Fatalf("stale redis client should already be closed, got %v", err)
	}
}
