package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	activeReceiptStore     ReceiptStore = NewInMemoryReceiptStore()
	activeReceiptStoreMu   sync.RWMutex
	receiptCleanupInterval = 5 * time.Minute
)

type receiptEntry struct {
	receipt   *SignedReceipt
	expiresAt time.Time
}

type InMemoryReceiptStore struct {
	mu       sync.RWMutex
	receipts map[string]*receiptEntry
	now      func() time.Time
}

func NewInMemoryReceiptStore() *InMemoryReceiptStore {
	return &InMemoryReceiptStore{
		receipts: make(map[string]*receiptEntry),
		now:      time.Now,
	}
}

func (s *InMemoryReceiptStore) Store(ctx context.Context, receipt *SignedReceipt, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateReceipt(receipt); err != nil {
		return fmt.Errorf("invalid receipt format: %w", err)
	}
	if err := validateReceiptTTL(ttl); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.receipts[receipt.Receipt.ID] = &receiptEntry{
		receipt:   cloneSignedReceipt(receipt),
		expiresAt: s.now().Add(ttl),
	}

	return nil
}

func (s *InMemoryReceiptStore) Get(ctx context.Context, id string) (*SignedReceipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	s.mu.RLock()
	entry, exists := s.receipts[id]
	s.mu.RUnlock()
	if !exists {
		return nil, false, nil
	}

	if s.now().After(entry.expiresAt) {
		s.mu.Lock()
		if current, ok := s.receipts[id]; ok && s.now().After(current.expiresAt) {
			delete(s.receipts, id)
		}
		s.mu.Unlock()
		return nil, false, nil
	}

	return cloneSignedReceipt(entry.receipt), true, nil
}

func (s *InMemoryReceiptStore) CleanupExpired(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	now := s.now()
	count := 0

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, entry := range s.receipts {
		if now.After(entry.expiresAt) {
			delete(s.receipts, id)
			count++
		}
	}

	if count > 0 {
		log.Printf("Cleaned up %d expired receipts", count)
	}

	return nil
}

func (s *InMemoryReceiptStore) Close() error {
	return nil
}

func cloneSignedReceipt(receipt *SignedReceipt) *SignedReceipt {
	if receipt == nil {
		return nil
	}
	clone := *receipt
	return &clone
}

func validateReceiptTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("receipt ttl must be positive")
	}
	return nil
}

func getActiveReceiptStore() ReceiptStore {
	activeReceiptStoreMu.RLock()
	defer activeReceiptStoreMu.RUnlock()
	return activeReceiptStore
}

func setActiveReceiptStore(store ReceiptStore) {
	activeReceiptStoreMu.Lock()
	defer activeReceiptStoreMu.Unlock()
	activeReceiptStore = store
}

func initReceiptStore() error {
	var store ReceiptStore
	var err error

	switch getReceiptStoreMode() {
	case receiptStoreModeMemory:
		store = NewInMemoryReceiptStore()
	case receiptStoreModeRedis:
		store, err = NewRedisReceiptStore(redisClient)
	default:
		return fmt.Errorf("unsupported receipt store mode %q", getReceiptStoreMode())
	}
	if err != nil {
		return err
	}

	setActiveReceiptStore(store)
	log.Printf("Receipt store initialized: %s", getReceiptStoreMode())
	return nil
}

// startReceiptCleanup runs periodic cleanup in a single goroutine.
func startReceiptCleanup(ctx context.Context) {
	ticker := time.NewTicker(receiptCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Receipt cleanup goroutine stopped")
			return
		case <-ticker.C:
			cleanupExpiredReceipts(ctx)
		}
	}
}

func cleanupExpiredReceipts(ctx context.Context) {
	if err := getActiveReceiptStore().CleanupExpired(ctx); err != nil {
		log.Printf("Failed to cleanup expired receipts: %v", err)
	}
}

func storeReceipt(receipt *SignedReceipt, ttl time.Duration) error {
	return storeReceiptWithContext(context.Background(), receipt, ttl)
}

func storeReceiptWithContext(ctx context.Context, receipt *SignedReceipt, ttl time.Duration) error {
	return getActiveReceiptStore().Store(ctx, receipt, ttl)
}

func getReceipt(id string) (*SignedReceipt, bool) {
	receipt, exists, err := getReceiptWithContext(context.Background(), id)
	if err != nil {
		log.Printf("Failed to retrieve receipt %s: %v", id, err)
		return nil, false
	}
	return receipt, exists
}

func getReceiptWithContext(ctx context.Context, id string) (*SignedReceipt, bool, error) {
	return getActiveReceiptStore().Get(ctx, id)
}
