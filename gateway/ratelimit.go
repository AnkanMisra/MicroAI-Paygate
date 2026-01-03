package main

import (
	"math"
	"sync"
	"time"
)

// RateLimiter defines the interface for rate limiting implementations
type RateLimiter interface {
	// Allow checks if a single request is allowed for the given key
	Allow(key string) bool
	// AllowN checks if N requests are allowed for the given key (for future bulk operations)
	AllowN(key string, n int) bool
	// GetRemaining returns the number of remaining tokens for the given key
	GetRemaining(key string) int
	// GetResetTime returns the Unix timestamp when the bucket will be fully refilled
	GetResetTime(key string) int64
}

// bucket represents a single token bucket for a user/IP
type bucket struct {
	tokens    float64   // Current number of tokens
	lastCheck time.Time // Last time tokens were refilled
	mu        sync.Mutex
}

// TokenBucket implements the token bucket rate limiting algorithm
type TokenBucket struct {
	rate       float64       // Tokens added per second
	burst      int           // Maximum tokens in bucket
	buckets    sync.Map      // map[string]*bucket - thread-safe map of user buckets
	cleanupTTL time.Duration // Time after which inactive buckets are cleaned up
	stopCh     chan struct{} // Channel to stop cleanup goroutine
}

// NewTokenBucket creates a new TokenBucket rate limiter
// rpm: requests per minute
// burst: maximum burst size (max tokens)
// cleanupTTL: duration after which inactive buckets are removed
func NewTokenBucket(rpm int, burst int, cleanupTTL time.Duration) *TokenBucket {
	if rpm <= 0 {
		rpm = 1
	}
	if burst <= 0 {
		burst = 1
	}
	
	tb := &TokenBucket{
		rate:       float64(rpm) / 60.0,
		burst:      burst,
		cleanupTTL: cleanupTTL,
		stopCh:     make(chan struct{}),
	}
	
	go tb.cleanup()
	
	return tb
}

// getBucket retrieves or creates a bucket for the given key
func (tb *TokenBucket) getBucket(key string) *bucket {
	// Use LoadOrStore to atomically get existing or create new bucket
	// This prevents race conditions where two goroutines might create separate buckets
	newBucket := &bucket{
		tokens:    float64(tb.burst),
		lastCheck: time.Now(),
	}
	
	val, _ := tb.buckets.LoadOrStore(key, newBucket)
	return val.(*bucket)
}

// Allow checks if a single request is allowed and consumes a token if available
func (tb *TokenBucket) Allow(key string) bool {
	return tb.AllowN(key, 1)
}

// AllowN checks if N requests are allowed and consumes N tokens if available
func (tb *TokenBucket) AllowN(key string, n int) bool {
	b := tb.getBucket(key)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now

	// Refill tokens based on elapsed time
	b.tokens = math.Min(float64(tb.burst), b.tokens+elapsed*tb.rate)

	// Check if enough tokens are available
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}

	return false
}

// GetRemaining returns the number of remaining tokens for the given key
func (tb *TokenBucket) GetRemaining(key string) int {
	val, ok := tb.buckets.Load(key)
	if !ok {
		return tb.burst
	}
	
	b := val.(*bucket)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	tokens := math.Min(float64(tb.burst), b.tokens+elapsed*tb.rate)

	return int(math.Floor(tokens))
}

// GetResetTime returns the Unix timestamp when the bucket will be fully refilled
func (tb *TokenBucket) GetResetTime(key string) int64 {
	val, ok := tb.buckets.Load(key)
	if !ok {
		return time.Now().Unix()
	}
	
	b := val.(*bucket)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	currentTokens := math.Min(float64(tb.burst), b.tokens+elapsed*tb.rate)

	tokensNeeded := float64(tb.burst) - currentTokens
	if tokensNeeded <= 0 {
		return now.Unix()
	}

	secondsToFull := tokensNeeded / tb.rate
	resetTime := now.Add(time.Duration(secondsToFull * float64(time.Second)))

	return resetTime.Unix()
}

// cleanup runs in a background goroutine to remove stale buckets
// This prevents memory leaks from inactive users
func (tb *TokenBucket) Stop() {
	close(tb.stopCh)
}

func (tb *TokenBucket) cleanup() {
	ticker := time.NewTicker(tb.cleanupTTL)
	defer ticker.Stop()

	for {
		select {
		case <-tb.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			tb.buckets.Range(func(key, value interface{}) bool {
				b := value.(*bucket)
				b.mu.Lock()
				lastCheck := b.lastCheck
				b.mu.Unlock()

				if now.Sub(lastCheck) > tb.cleanupTTL {
					tb.buckets.Delete(key)
				}
				return true
			})
		}
	}
}
