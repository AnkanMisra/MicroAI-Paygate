package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestGenerateReceiptID(t *testing.T) {
	// Generate multiple IDs and check format
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id, err := generateReceiptID()
		if err != nil {
			t.Fatalf("generateReceiptID() failed: %v", err)
		}

		// Check format
		if !strings.HasPrefix(id, "rcpt_") {
			t.Errorf("Receipt ID should start with 'rcpt_', got: %s", id)
		}

		// Check length (rcpt_ + 12 hex chars = 17 total)
		if len(id) != 17 {
			t.Errorf("Receipt ID should be 17 characters, got %d: %s", len(id), id)
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("Duplicate receipt ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestHashData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "Empty data",
			data:     []byte{},
			expected: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "Simple text",
			data:     []byte("test"),
			expected: "sha256:" + hashHex([]byte("test")),
		},
		{
			name:     "JSON data",
			data:     []byte(`{"key":"value"}`),
			expected: "sha256:" + hashHex([]byte(`{"key":"value"}`)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashData(tt.data)
			if result != tt.expected {
				t.Errorf("hashData() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSignReceipt(t *testing.T) {
	// Create a test receipt
	receipt := Receipt{
		ID:        "rcpt_test123456",
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
		Payment: PaymentDetails{
			Payer:     "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21",
			Recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
			Amount:    "0.001",
			Token:     "USDC",
			ChainID:   8453,
			Nonce:     "test-nonce-123",
		},
		Service: ServiceDetails{
			Endpoint:     "/api/ai/summarize",
			RequestHash:  "sha256:abc123",
			ResponseHash: "sha256:def456",
		},
	}

	// This test requires SERVER_WALLET_PRIVATE_KEY to be set
	// Skip if not available
	privateKey, err := getServerPrivateKey()
	if err != nil || privateKey == nil {
		t.Skip("Skipping signature test: SERVER_WALLET_PRIVATE_KEY not set")
	}

	signedReceipt, err := signReceipt(receipt)
	if err != nil {
		t.Fatalf("Failed to sign receipt: %v", err)
	}

	// Verify signature format
	if !strings.HasPrefix(signedReceipt.Signature, "0x") {
		t.Error("Signature should start with '0x'")
	}

	// Verify server public key format
	if !strings.HasPrefix(signedReceipt.ServerPublicKey, "0x") {
		t.Error("ServerPublicKey should start with '0x'")
	}

	// Verify receipt is intact
	if signedReceipt.Receipt.ID != receipt.ID {
		t.Error("Receipt ID mismatch after signing")
	}
}

func TestReceiptJSONSerialization(t *testing.T) {
	receipt := Receipt{
		ID:        "rcpt_abc123def456",
		Version:   "1.0",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Payment: PaymentDetails{
			Payer:     "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21",
			Recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
			Amount:    "0.001",
			Token:     "USDC",
			ChainID:   8453,
			Nonce:     "test-nonce",
		},
		Service: ServiceDetails{
			Endpoint:     "/api/ai/summarize",
			RequestHash:  "sha256:request",
			ResponseHash: "sha256:response",
		},
	}

	// Serialize twice to check determinism
	json1, err1 := json.Marshal(receipt)
	json2, err2 := json.Marshal(receipt)

	if err1 != nil || err2 != nil {
		t.Fatalf("JSON marshaling failed: %v, %v", err1, err2)
	}

	if string(json1) != string(json2) {
		t.Error("JSON serialization is not deterministic")
	}

	// Verify all fields are present
	var decoded map[string]interface{}
	if err := json.Unmarshal(json1, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON for field verification: %v", err)
	}

	requiredFields := []string{"id", "version", "timestamp", "payment", "service"}
	for _, field := range requiredFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("Missing field in JSON: %s", field)
		}
	}
}

func TestStoreAndRetrieveReceipt(t *testing.T) {
	receiptID, err := generateReceiptID()
	if err != nil {
		t.Fatalf("generateReceiptID() failed: %v", err)
	}

	signedReceipt := &SignedReceipt{
		Receipt: Receipt{
			ID:        receiptID,
			Version:   "1.0",
			Timestamp: time.Now().UTC(),
			Payment: PaymentDetails{
				Payer:     "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21",
				Recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
				Amount:    "0.001",
				Token:     "USDC",
				ChainID:   8453,
				Nonce:     "test-nonce",
			},
			Service: ServiceDetails{
				Endpoint:     "/api/ai/summarize",
				RequestHash:  "sha256:test",
				ResponseHash: "sha256:response",
			},
		},
		Signature:       "0x1234567890abcdef",
		ServerPublicKey: "0xabcdef1234567890",
	}

	// Store receipt
	if err := storeReceipt(signedReceipt, 24*time.Hour); err != nil {
		t.Fatalf("Failed to store receipt: %v", err)
	}

	// Retrieve receipt
	retrieved, exists := getReceipt(signedReceipt.Receipt.ID)
	if !exists {
		t.Fatal("Receipt not found after storing")
	}

	if retrieved.Receipt.ID != signedReceipt.Receipt.ID {
		t.Error("Retrieved receipt ID doesn't match stored receipt")
	}

	if retrieved.Signature != signedReceipt.Signature {
		t.Error("Retrieved receipt signature doesn't match")
	}
}

func TestReceiptNotFound(t *testing.T) {
	_, exists := getReceipt("rcpt_nonexistent")
	if exists {
		t.Error("Non-existent receipt should not be found")
	}
}

func TestHashDataConsistency(t *testing.T) {
	data := []byte("consistent test data")

	// Hash multiple times
	hash1 := hashData(data)
	hash2 := hashData(data)
	hash3 := hashData(data)

	if hash1 != hash2 || hash2 != hash3 {
		t.Error("hashData should produce consistent results")
	}
}

// Helper function for testing
func hashHex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func TestVerifyReceiptSignature(t *testing.T) {
	// This test verifies that signature verification works correctly
	// Skip if private key not available
	privateKey, err := getServerPrivateKey()
	if err != nil || privateKey == nil {
		t.Skip("Skipping verification test: SERVER_WALLET_PRIVATE_KEY not set")
	}

	receiptID, err := generateReceiptID()
	if err != nil {
		t.Fatalf("generateReceiptID() failed: %v", err)
	}

	receipt := Receipt{
		ID:        receiptID,
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
		Payment: PaymentDetails{
			Payer:     "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21",
			Recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
			Amount:    "0.001",
			Token:     "USDC",
			ChainID:   8453,
			Nonce:     "test-nonce-verification",
		},
		Service: ServiceDetails{
			Endpoint:     "/api/ai/summarize",
			RequestHash:  "sha256:testrequest",
			ResponseHash: "sha256:testresponse",
		},
	}

	signedReceipt, err := signReceipt(receipt)
	if err != nil {
		t.Fatalf("Failed to sign receipt: %v", err)
	}

	// Manually verify the signature using crypto.VerifySignature
	// This is more robust than SigToPub as it doesn't rely on recovery ID
	receiptBytes, _ := json.Marshal(signedReceipt.Receipt)
	hash := crypto.Keccak256Hash(receiptBytes)

	// Remove "0x" prefix from signature
	sigHex := signedReceipt.Signature[2:]
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatalf("Failed to decode signature: %v", err)
	}

	// Get server's public key bytes
	serverPubBytes := crypto.FromECDSAPub(&privateKey.PublicKey)

	// Verify signature without recovery ID (remove last byte which is the recovery ID)
	// SECURITY: crypto.VerifySignature uses constant-time comparison to prevent timing attacks
	if !crypto.VerifySignature(serverPubBytes, hash.Bytes(), sigBytes[:64]) {
		t.Error("Signature verification failed")
	}
}

func TestReceiptFullFlowIntegration(t *testing.T) {
	// Integration test for complete receipt lifecycle:
	// 1. Generate receipt
	// 2. Store with TTL
	// 3. Retrieve by ID
	// 4. Verify signature
	// 5. Verify expiration

	// Skip if private key not available
	privateKey, err := getServerPrivateKey()
	if err != nil || privateKey == nil {
		t.Skip("Skipping integration test: SERVER_WALLET_PRIVATE_KEY not set")
	}

	// Step 1: Create mock payment context and data
	paymentCtx := PaymentContext{
		Recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
		Token:     "USDC",
		Amount:    "0.001",
		Nonce:     "integration-test-nonce",
		ChainID:   8453,
	}

	payer := "0x742d35Cc6634C0532925a3b844Bc9e7595f8fE21"
	endpoint := "/api/ai/summarize"
	requestBody := []byte(`{"text":"Test input for summarization"}`)
	responseBody := []byte(`This is a test AI response summary.`)

	// Step 2: Generate receipt (simulates what happens in handleSummarize)
	receipt, err := GenerateReceipt(paymentCtx, payer, endpoint, requestBody, responseBody)
	if err != nil {
		t.Fatalf("Failed to generate receipt: %v", err)
	}

	// Verify receipt structure
	if receipt.Receipt.ID == "" || !strings.HasPrefix(receipt.Receipt.ID, "rcpt_") {
		t.Errorf("Invalid receipt ID: %s", receipt.Receipt.ID)
	}
	if receipt.Receipt.Payment.Payer != payer {
		t.Errorf("Payer mismatch: got %s, want %s", receipt.Receipt.Payment.Payer, payer)
	}
	if receipt.Receipt.Payment.Amount != "0.001" {
		t.Errorf("Amount mismatch: got %s, want 0.001", receipt.Receipt.Payment.Amount)
	}
	if receipt.Signature == "" {
		t.Error("Receipt signature is empty")
	}
	if receipt.ServerPublicKey == "" {
		t.Error("Server public key is empty")
	}

	// Verify hashes are present
	if !strings.HasPrefix(receipt.Receipt.Service.RequestHash, "sha256:") {
		t.Errorf("Invalid request hash format: %s", receipt.Receipt.Service.RequestHash)
	}
	if !strings.HasPrefix(receipt.Receipt.Service.ResponseHash, "sha256:") {
		t.Errorf("Invalid response hash format: %s", receipt.Receipt.Service.ResponseHash)
	}

	// Step 3: Store receipt with TTL
	ttl := 1 * time.Hour
	receiptID := receipt.Receipt.ID

	if err := storeReceipt(receipt, ttl); err != nil {
		t.Fatalf("Failed to store receipt: %v", err)
	}

	// Step 4: Retrieve receipt by ID (simulates GET /api/receipts/:id)
	retrievedReceipt, exists := getReceipt(receiptID)
	if !exists {
		t.Fatal("Receipt not found after storage")
	}

	// Verify retrieved receipt matches original
	if retrievedReceipt.Receipt.ID != receipt.Receipt.ID {
		t.Errorf("Receipt ID mismatch: got %s, want %s", retrievedReceipt.Receipt.ID, receipt.Receipt.ID)
	}
	if retrievedReceipt.Signature != receipt.Signature {
		t.Error("Signature mismatch after retrieval")
	}
	if retrievedReceipt.Receipt.Payment.Nonce != paymentCtx.Nonce {
		t.Errorf("Nonce mismatch: got %s, want %s", retrievedReceipt.Receipt.Payment.Nonce, paymentCtx.Nonce)
	}

	// Step 5: Verify signature (simulates client-side verification)
	receiptBytes, err := json.Marshal(retrievedReceipt.Receipt)
	if err != nil {
		t.Fatalf("Failed to marshal retrieved receipt: %v", err)
	}

	hash := crypto.Keccak256Hash(receiptBytes)

	// Decode signature
	sigHex := retrievedReceipt.Signature[2:] // Remove 0x prefix
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		t.Fatalf("Failed to decode signature: %v", err)
	}

	// Verify signature
	serverPubBytes := crypto.FromECDSAPub(&privateKey.PublicKey)
	if !crypto.VerifySignature(serverPubBytes, hash.Bytes(), sigBytes[:64]) {
		t.Error("Signature verification failed for retrieved receipt")
	}

	// Step 6: Verify expiration behavior
	// Store a receipt with very short TTL
	shortTTLReceipt, err := GenerateReceipt(paymentCtx, payer, endpoint, requestBody, responseBody)
	if err != nil {
		t.Fatalf("Failed to generate short TTL receipt: %v", err)
	}

	shortTTL := 100 * time.Millisecond
	if err := storeReceipt(shortTTLReceipt, shortTTL); err != nil {
		t.Fatalf("Failed to store short TTL receipt: %v", err)
	}

	shortTTLID := shortTTLReceipt.Receipt.ID

	// Verify it exists immediately
	if _, exists := getReceipt(shortTTLID); !exists {
		t.Error("Short TTL receipt should exist immediately after storage")
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Verify it's expired
	if _, exists := getReceipt(shortTTLID); exists {
		t.Error("Short TTL receipt should be expired after waiting")
	}

	// Step 7: Test validation
	// Create an invalid receipt (missing required field)
	invalidReceipt := &SignedReceipt{
		Receipt: Receipt{
			ID:      "", // Invalid: empty ID
			Version: "1.0",
		},
		Signature:       "0x1234",
		ServerPublicKey: "0x5678",
	}

	// Should fail validation
	if err := storeReceipt(invalidReceipt, ttl); err == nil {
		t.Error("Expected error when storing invalid receipt, got nil")
	}

	t.Log("Integration test completed successfully:")
	t.Logf("  - Generated receipt with ID: %s", receiptID)
	t.Logf("  - Stored and retrieved successfully")
	t.Logf("  - Signature verified")
	t.Logf("  - Expiration working correctly")
	t.Logf("  - Validation working correctly")
}
