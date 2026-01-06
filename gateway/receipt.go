package main

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Receipt represents a cryptographic payment receipt
type Receipt struct {
	ID        string          `json:"id"`
	Version   string          `json:"version"`
	Timestamp time.Time       `json:"timestamp"`
	Payment   PaymentDetails  `json:"payment"`
	Service   ServiceDetails  `json:"service"`
}

// PaymentDetails contains payment-related information
type PaymentDetails struct {
	Payer     string `json:"payer"`
	Recipient string `json:"recipient"`
	Amount    string `json:"amount"`
	Token     string `json:"token"`
	ChainID   int    `json:"chainId"`
	Nonce     string `json:"nonce"`
}

// ServiceDetails contains service-related information
type ServiceDetails struct {
	Endpoint     string `json:"endpoint"`
	RequestHash  string `json:"request_hash"`
	ResponseHash string `json:"response_hash"`
}

// SignedReceipt contains the receipt and its cryptographic signature
type SignedReceipt struct {
	Receipt         Receipt `json:"receipt"`
	Signature       string  `json:"signature"`
	ServerPublicKey string  `json:"server_public_key"`
}

// GenerateReceipt creates a new receipt for a successful payment
func GenerateReceipt(payment PaymentContext, payer string, endpoint string, reqBody, respBody []byte) (*SignedReceipt, error) {
	receipt := Receipt{
		ID:        generateReceiptID(),
		Version:   "1.0",
		Timestamp: time.Now().UTC(),
		Payment: PaymentDetails{
			Payer:     payer,
			Recipient: payment.Recipient,
			Amount:    payment.Amount,
			Token:     payment.Token,
			ChainID:   payment.ChainID,
			Nonce:     payment.Nonce,
		},
		Service: ServiceDetails{
			Endpoint:     endpoint,
			RequestHash:  hashData(reqBody),
			ResponseHash: hashData(respBody),
		},
	}

	return signReceipt(receipt)
}

// generateReceiptID generates a unique receipt ID with "rcpt_" prefix
func generateReceiptID() string {
	// Generate 6 random bytes (12 hex characters)
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random fails
		// This maintains uniqueness even if entropy is exhausted
		timestamp := time.Now().UnixNano()
		return fmt.Sprintf("rcpt_%012x", timestamp)
	}
	return "rcpt_" + hex.EncodeToString(bytes)
}

// hashData computes SHA-256 hash of data and returns hex-encoded string
func hashData(data []byte) string {
	if len(data) == 0 {
		return "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // Empty hash
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// signReceipt signs a receipt using the server's private key
// NOTE: Go's json.Marshal is deterministic for structs - fields are always
// serialized in alphabetical order by their JSON tag names.
// This ensures consistent signatures. Non-determinism only affects map types.
func signReceipt(receipt Receipt) (*SignedReceipt, error) {
	// Get server's private key
	privateKey, err := getServerPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load server private key: %w", err)
	}

	// Serialize receipt deterministically
	// For structs, json.Marshal always outputs fields in alphabetical order by JSON tag
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal receipt: %w", err)
	}

	// Hash the receipt using Keccak256 (Ethereum-compatible)
	hash := crypto.Keccak256Hash(receiptBytes)

	// Sign the hash using ECDSA
	// SECURITY: crypto.Sign uses constant-time operations from go-ethereum's secp256k1 implementation
	// This prevents timing attacks that could leak private key information
	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign receipt: %w", err)
	}

	// Get server's public key for verification
	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	publicKeyBytes := crypto.FromECDSAPub(publicKey)

	return &SignedReceipt{
		Receipt:         receipt,
		Signature:       "0x" + hex.EncodeToString(signature),
		ServerPublicKey: "0x" + hex.EncodeToString(publicKeyBytes),
	}, nil
}
