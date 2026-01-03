package main

import (
	"sync"
	"testing"
	"time"
)

// TestTokenBucketAllow tests basic token consumption
func TestTokenBucketAllow(t *testing.T) {
	tb := NewTokenBucket(60, 5, 5*time.Minute) // 60 RPM (1 per second), burst of 5
	defer stopCleanup(tb)

	key := "test-user"

	// Burst should allow first 5 requests immediately
	for i := 0; i < 5; i++ {
		if !tb.Allow(key) {
			t.Errorf("Request %d should be allowed (burst)", i+1)
		}
	}

	// 6th request should be denied (burst exhausted)
	if tb.Allow(key) {
		t.Error("Request should be denied after burst exhausted")
	}
}

// TestTokenBucketRefill tests token refill over time
func TestTokenBucketRefill(t *testing.T) {
	tb := NewTokenBucket(60, 3, 5*time.Minute) // 60 RPM = 1 token per second
	defer stopCleanup(tb)

	key := "test-refill"

	// Consume all tokens
	for i := 0; i < 3; i++ {
		tb.Allow(key)
	}

	// Should be denied immediately
	if tb.Allow(key) {
		t.Error("Request should be denied (no tokens)")
	}

	// Wait for 1 second to refill 1 token
	time.Sleep(1100 * time.Millisecond) // Add buffer for timing

	// Should now allow 1 request
	if !tb.Allow(key) {
		t.Error("Request should be allowed after refill")
	}

	// Should be denied again
	if tb.Allow(key) {
		t.Error("Request should be denied after consuming refilled token")
	}
}

// TestTokenBucketConcurrency tests thread safety
func TestTokenBucketConcurrency(t *testing.T) {
	tb := NewTokenBucket(600, 100, 5*time.Minute) // High limits for concurrency test
	defer stopCleanup(tb)

	key := "test-concurrent"
	concurrency := 50
	requestsPerGoroutine := 2

	var wg sync.WaitGroup
	allowed := make(chan bool, concurrency*requestsPerGoroutine)

	// Launch concurrent requests
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				allowed <- tb.Allow(key)
			}
		}()
	}

	wg.Wait()
	close(allowed)

	// Count allowed requests
	allowedCount := 0
	for a := range allowed {
		if a {
			allowedCount++
		}
	}

	// Should allow exactly burst amount (100)
	if allowedCount != 100 {
		t.Errorf("Expected exactly 100 allowed requests, got %d", allowedCount)
	}
}

// TestTokenBucketGetRemaining tests remaining token calculation
func TestTokenBucketGetRemaining(t *testing.T) {
	tb := NewTokenBucket(60, 10, 5*time.Minute)
	defer stopCleanup(tb)

	key := "test-remaining"

	// Initially should have full burst
	remaining := tb.GetRemaining(key)
	if remaining != 10 {
		t.Errorf("Expected 10 remaining tokens, got %d", remaining)
	}

	// Consume 3 tokens
	for i := 0; i < 3; i++ {
		tb.Allow(key)
	}

	remaining = tb.GetRemaining(key)
	if remaining != 7 {
		t.Errorf("Expected 7 remaining tokens, got %d", remaining)
	}
}

// TestTokenBucketResetTime tests reset time calculation
func TestTokenBucketResetTime(t *testing.T) {
	tb := NewTokenBucket(60, 5, 5*time.Minute) // 1 token/second
	defer stopCleanup(tb)

	key := "test-reset"

	// Consume all tokens
	for i := 0; i < 5; i++ {
		tb.Allow(key)
	}

	resetTime := tb.GetResetTime(key)
	now := time.Now().Unix()

	// Reset should be approximately 5 seconds in the future (5 tokens * 1 second each)
	diff := resetTime - now
	if diff < 4 || diff > 6 {
		t.Errorf("Expected reset time ~5 seconds from now, got %d", diff)
	}
}

// TestTokenBucketAllowN tests bulk token consumption
func TestTokenBucketAllowN(t *testing.T) {
	tb := NewTokenBucket(60, 10, 5*time.Minute)
	defer stopCleanup(tb)

	key := "test-allowN"

	// Should allow consuming 5 tokens at once
	if !tb.AllowN(key, 5) {
		t.Error("Should allow consuming 5 tokens")
	}

	// Should have 5 remaining
	remaining := tb.GetRemaining(key)
	if remaining != 5 {
		t.Errorf("Expected 5 remaining tokens, got %d", remaining)
	}

	// Should allow consuming 5 more
	if !tb.AllowN(key, 5) {
		t.Error("Should allow consuming remaining 5 tokens")
	}

	// Should not allow consuming 1 more (empty)
	if tb.AllowN(key, 1) {
		t.Error("Should not allow consuming tokens when empty")
	}
}

// TestTokenBucketMultipleKeys tests isolation between different keys
func TestTokenBucketMultipleKeys(t *testing.T) {
	tb := NewTokenBucket(60, 5, 5*time.Minute)
	defer stopCleanup(tb)

	key1 := "user1"
	key2 := "user2"

	// Exhaust tokens for key1
	for i := 0; i < 5; i++ {
		tb.Allow(key1)
	}

	// key1 should be denied
	if tb.Allow(key1) {
		t.Error("key1 should be denied")
	}

	// key2 should still be allowed (separate bucket)
	if !tb.Allow(key2) {
		t.Error("key2 should be allowed (separate bucket)")
	}
}

// TestTokenBucketCleanup tests that stale buckets are cleaned up
func TestTokenBucketCleanup(t *testing.T) {
	// Use short cleanup TTL for testing
	tb := NewTokenBucket(60, 5, 100*time.Millisecond)
	defer stopCleanup(tb)

	key := "test-cleanup"

	// Use the bucket
	tb.Allow(key)

	// Verify bucket exists
	_, exists := tb.buckets.Load(key)
	if !exists {
		t.Error("Bucket should exist after use")
	}

	// Wait for cleanup to run (TTL + buffer)
	time.Sleep(250 * time.Millisecond)

	// Verify bucket was cleaned up
	_, exists = tb.buckets.Load(key)
	if exists {
		t.Error("Bucket should be cleaned up after TTL")
	}
}

// stopCleanup stops the cleanup goroutine by deleting all buckets
// This is a helper to prevent goroutine leaks in tests
func stopCleanup(tb *TokenBucket) {
	tb.buckets.Range(func(key, value interface{}) bool {
		tb.buckets.Delete(key)
		return true
	})
}

// BenchmarkTokenBucketAllow benchmarks the Allow method
func BenchmarkTokenBucketAllow(b *testing.B) {
	tb := NewTokenBucket(6000, 1000, 5*time.Minute) // High limits
	defer stopCleanup(tb)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Allow("benchmark-user")
	}
}

// BenchmarkTokenBucketConcurrent benchmarks concurrent access
func BenchmarkTokenBucketConcurrent(b *testing.B) {
	tb := NewTokenBucket(60000, 10000, 5*time.Minute) // Very high limits
	defer stopCleanup(tb)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tb.Allow("benchmark-concurrent")
		}
	})
}
