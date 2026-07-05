package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"gateway/internal/ai"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var (
	serverPrivateKeyTestMu sync.Mutex
	receiptGlobalsTestMu   sync.Mutex
)

func TestRedisReceiptStore_PersistsAcrossGatewayRestart(t *testing.T) {
	ctx := t.Context()
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer rdb.Close()

	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(VerifyResponse{
			IsValid:          true,
			RecoveredAddress: "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21",
		})
	}))
	defer verifier.Close()

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Redis receipt summary"}}]}`))
	}))
	defer aiServer.Close()

	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "redis")
	t.Setenv("REDIS_URL", redisServer.Addr())
	t.Setenv("AI_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_URL", aiServer.URL)
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("VERIFIER_URL", verifier.URL)
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")
	t.Setenv("RECEIPT_TTL", "86400")

	resetServerPrivateKeyForTest(t)
	restoreReceiptGlobals := replaceReceiptGlobalsForTest(t)
	defer restoreReceiptGlobals()

	if err := initRedis(); err != nil {
		t.Fatalf("init redis: %v", err)
	}
	if err := initReceiptStore(); err != nil {
		t.Fatalf("init redis receipt store: %v", err)
	}

	var err error
	aiProvider, err = ai.NewProvider()
	if err != nil {
		t.Fatalf("new AI provider: %v", err)
	}

	firstGateway := newReceiptPersistenceTestRouter()
	createReq := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", bytes.NewBufferString(`{"text":"persist this receipt"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-402-Signature", "0xValidSig")
	createReq.Header.Set("X-402-Nonce", "restart-test-nonce")
	createReq.Header.Set("X-402-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	createResp := httptest.NewRecorder()
	firstGateway.ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusOK {
		t.Fatalf("create receipt status=%d body=%s", createResp.Code, createResp.Body.String())
	}

	var receiptBase64 string
	lines := strings.Split(createResp.Body.String(), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
				if r, ok := chunk["receipt"].(string); ok {
					receiptBase64 = r
				}
			}
		}
	}

	if receiptBase64 == "" {
		t.Fatal("missing receipt event in SSE stream")
	}
	receiptJSON, err := base64.StdEncoding.DecodeString(receiptBase64)
	if err != nil {
		t.Fatalf("decode receipt header: %v", err)
	}
	var created SignedReceipt
	if err := json.Unmarshal(receiptJSON, &created); err != nil {
		t.Fatalf("unmarshal receipt header: %v", err)
	}
	t.Cleanup(func() {
		_ = rdb.Del(ctx, redisReceiptKey(created.Receipt.ID)).Err()
	})

	// Simulate a gateway restart by rebuilding Redis and receipt-store globals
	// from environment config before routing the lookup through a fresh engine.
	if redisClient != nil {
		_ = redisClient.Close()
		redisClient = nil
	}
	if err := initRedis(); err != nil {
		t.Fatalf("restart init redis: %v", err)
	}
	if err := initReceiptStore(); err != nil {
		t.Fatalf("restart init receipt store: %v", err)
	}

	secondGateway := newReceiptPersistenceTestRouter()
	lookupReq := httptest.NewRequest(http.MethodGet, "/api/receipts/"+created.Receipt.ID, nil)
	lookupResp := httptest.NewRecorder()
	secondGateway.ServeHTTP(lookupResp, lookupReq)

	if lookupResp.Code != http.StatusOK {
		t.Fatalf("lookup receipt status=%d body=%s", lookupResp.Code, lookupResp.Body.String())
	}
	var lookup map[string]any
	if err := json.Unmarshal(lookupResp.Body.Bytes(), &lookup); err != nil {
		t.Fatalf("unmarshal lookup response: %v", err)
	}
	receiptValue, ok := lookup["receipt"]
	if !ok {
		t.Fatalf("lookup response missing receipt field: %v", lookup)
	}
	receiptBody, ok := receiptValue.(map[string]any)
	if !ok {
		t.Fatalf("lookup receipt has unexpected type: %T", receiptValue)
	}
	receiptID, ok := receiptBody["id"].(string)
	if !ok {
		t.Fatalf("lookup receipt id has unexpected type: %T", receiptBody["id"])
	}
	if receiptID != created.Receipt.ID {
		t.Fatalf("lookup receipt ID mismatch: got %v, want %s", receiptID, created.Receipt.ID)
	}
}

func newReceiptPersistenceTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/ai/summarize", handleSummarize)
	r.GET("/api/receipts/:id", handleGetReceipt)
	return r
}

func resetServerPrivateKeyForTest(t *testing.T) {
	t.Helper()
	serverPrivateKeyTestMu.Lock()
	origKey := serverPrivateKey
	origErr := serverPrivateKeyErr
	serverPrivateKey = nil
	serverPrivateKeyErr = nil
	serverPrivateKeyOnce = sync.Once{}
	t.Cleanup(func() {
		serverPrivateKey = origKey
		serverPrivateKeyErr = origErr
		serverPrivateKeyOnce = sync.Once{}
		serverPrivateKeyTestMu.Unlock()
	})
}

func replaceReceiptGlobalsForTest(t *testing.T) func() {
	t.Helper()
	receiptGlobalsTestMu.Lock()
	origRedisClient := redisClient
	origStore := getActiveReceiptStore()
	origAIProvider := aiProvider
	return func() {
		if redisClient != nil && redisClient != origRedisClient {
			_ = redisClient.Close()
		}
		redisClient = origRedisClient
		setActiveReceiptStore(origStore)
		aiProvider = origAIProvider
		receiptGlobalsTestMu.Unlock()
	}
}

func TestHandleSummarize_ConcurrentReplayAttack(t *testing.T) {
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer rdb.Close()

	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock verifier that always approves
		_ = json.NewEncoder(w).Encode(VerifyResponse{
			IsValid:          true,
			RecoveredAddress: "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21",
		})
	}))
	defer verifier.Close()

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"AI response"}}]}`))
	}))
	defer aiServer.Close()

	t.Setenv("CACHE_ENABLED", "false")
	t.Setenv("RECEIPT_STORE", "redis")
	t.Setenv("REDIS_URL", redisServer.Addr())
	t.Setenv("AI_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_URL", aiServer.URL)
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("VERIFIER_URL", verifier.URL)
	t.Setenv("SERVER_WALLET_PRIVATE_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("RECIPIENT_ADDRESS", "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219")
	t.Setenv("RECEIPT_TTL", "86400")

	resetServerPrivateKeyForTest(t)
	restoreReceiptGlobals := replaceReceiptGlobalsForTest(t)
	defer restoreReceiptGlobals()

	if err := initRedis(); err != nil {
		t.Fatalf("init redis: %v", err)
	}
	if err := initReceiptStore(); err != nil {
		t.Fatalf("init redis receipt store: %v", err)
	}

	var err error
	aiProvider, err = ai.NewProvider()
	if err != nil {
		t.Fatalf("new AI provider: %v", err)
	}

	gateway := newReceiptPersistenceTestRouter()
	
	const numConcurrentRequests = 20
	var wg sync.WaitGroup
	wg.Add(numConcurrentRequests)

	successCount := 0
	conflictCount := 0
	var mu sync.Mutex

	// Create requests beforehand so they start at the exact same time
	var requests []*http.Request
	for i := 0; i < numConcurrentRequests; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/ai/summarize", bytes.NewBufferString(`{"text":"summarize me"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-402-Signature", "0xSameSignatureReplayTest") // Same signature for all requests
		req.Header.Set("X-402-Nonce", "replay-test-nonce")
		req.Header.Set("X-402-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
		requests = append(requests, req)
	}

	start := make(chan struct{})

	for i := 0; i < numConcurrentRequests; i++ {
		go func(req *http.Request) {
			defer wg.Done()
			<-start // wait for signal to start simultaneously
			
			resp := httptest.NewRecorder()
			gateway.ServeHTTP(resp, req)
			
			mu.Lock()
			if resp.Code == http.StatusOK {
				successCount++
			} else if resp.Code == http.StatusPaymentRequired {
				var body map[string]interface{}
				_ = json.Unmarshal(resp.Body.Bytes(), &body)
				if body["message"] == "Transaction already used" {
					conflictCount++
				}
			}
			mu.Unlock()
		}(requests[i])
	}
	
	close(start) // Signal all goroutines to start
	wg.Wait()
	
	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful request, got %d", successCount)
	}
	if conflictCount != numConcurrentRequests - 1 {
		t.Errorf("Expected exactly %d blocked replays, got %d", numConcurrentRequests - 1, conflictCount)
	}
}

