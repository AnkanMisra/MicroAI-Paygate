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
		id := generateReceiptID()
		
		// Check format
		if !strings.HasPrefix(id,  "rcpt_") {
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
	if serverPrivateKey == nil {
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
	json.Unmarshal(json1, &decoded)

	requiredFields := []string{"id", "version", "timestamp", "payment", "service"}
	for _, field := range requiredFields {
		if _, exists := decoded[field]; !exists {
			t.Errorf("Missing field in JSON: %s", field)
		}
	}
}

func TestStoreAndRetrieveReceipt(t *testing.T) {
	signedReceipt := &SignedReceipt{
		Receipt: Receipt{
			ID:        generateReceiptID(),
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
	storeReceipt(signedReceipt, 24*time.Hour)

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
	if serverPrivateKey == nil {
		t.Skip("Skipping verification test: SERVER_WALLET_PRIVATE_KEY not set")
	}

	receipt := Receipt{
		ID:        generateReceiptID(),
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

	// Manually verify the signature
	receiptBytes, _ := json.Marshal(signedReceipt.Receipt)
	hash := crypto.Keccak256Hash(receiptBytes)

	// Remove "0x" prefix from signature
	sigHex := signedReceipt.Signature[2:]
	sigBytes, _ := hex.DecodeString(sigHex)

	// Recover public key
	pubKey, err := crypto.SigToPub(hash.Bytes(), sigBytes)
	if err != nil {
		t.Fatalf("Failed to recover public key: %v", err)
	}

	// Compare with server's public key
	serverPubBytes := crypto.FromECDSAPub(&serverPrivateKey.PublicKey)
	recoveredPubBytes := crypto.FromECDSAPub(pubKey)

	if hex.EncodeToString(serverPubBytes) != hex.EncodeToString(recoveredPubBytes) {
		t.Error("Recovered public key doesn't match server's public key")
	}
}
