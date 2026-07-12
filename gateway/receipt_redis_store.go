package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisReceiptStore struct {
	client *redis.Client
}

func NewRedisReceiptStore(client *redis.Client) (*RedisReceiptStore, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	return &RedisReceiptStore{client: client}, nil
}

func (s *RedisReceiptStore) Store(ctx context.Context, receipt *SignedReceipt, ttl time.Duration) error {
	if err := validateReceipt(receipt); err != nil {
		return fmt.Errorf("invalid receipt format: %w", err)
	}
	if err := validateReceiptTTL(ttl); err != nil {
		return err
	}

	data, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("marshal receipt: %w", err)
	}

	if err := s.client.Set(ctx, redisReceiptKey(receipt.Receipt.ID), data, ttl).Err(); err != nil {
		return fmt.Errorf("store receipt in redis: %w", err)
	}
	return nil
}

func (s *RedisReceiptStore) Get(ctx context.Context, id string) (*SignedReceipt, bool, error) {
	data, err := s.client.Get(ctx, redisReceiptKey(id)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get receipt from redis: %w", err)
	}

	var receipt SignedReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, false, fmt.Errorf("decode receipt from redis: %w", err)
	}
	if err := validateReceipt(&receipt); err != nil {
		return nil, false, fmt.Errorf("invalid receipt in redis: %w", err)
	}

	return &receipt, true, nil
}

func (s *RedisReceiptStore) CleanupExpired(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *RedisReceiptStore) Close() error {
	return nil
}

func redisReceiptKey(id string) string {
	return "receipt:" + id
}
