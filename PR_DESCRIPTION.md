# Implement Cryptographic Payment Receipts

Closes #16

## Summary

Implemented cryptographic payment receipts that provide tamper-proof payment proofs for every successful API request. Each receipt is signed using ECDSA and includes payment details, service information, and can be independently verified by clients.

## Changes Made

### New Files

**`gateway/receipt.go`** (135 lines)
- `ReceiptStore` interface for future storage implementations (Redis/PostgreSQL)
- `Receipt` and `SignedReceipt` structs
- Receipt generation with ECDSA signing (Keccak256)
- SHA-256 hashing for request/response bodies
- Unique receipt ID generation (`rcpt_` + 12 hex chars)

**`gateway/receipt_test.go`** (301 lines)
- 10 comprehensive unit tests covering:
  - Receipt ID generation and uniqueness
  - SHA-256 hashing consistency
  - ECDSA signature generation
  - JSON serialization determinism
  - Storage and retrieval
  - Signature verification

### Modified Files

**`gateway/main.go`** (+140 lines)
- In-memory receipt storage with thread-safe access
- Automatic TTL-based cleanup (default 24h)
- Receipt generation after successful AI responses
- Added `X-402-Receipt` header to responses
- Included receipt in JSON response body
- New endpoint: `GET /api/receipts/:id` for receipt lookup
- `getServerPrivateKey()` - Load and cache server's private key
- Receipt management functions (store, get, TTL)

**`gateway/go.mod`**
- Added `github.com/ethereum/go-ethereum v1.14.12` for cryptographic operations

**`.env.example`**
- Added `SERVER_WALLET_PRIVATE_KEY` configuration
- Added `RECEIPT_TTL` configuration (default 86400 seconds / 24 hours)
- Documented future storage options (Redis, PostgreSQL)

## Implementation Details

### Architecture Decisions

**1. JSON Determinism**
- Go's `json.Marshal` is deterministic for structs (fields serialized alphabetically)
- Added comments clarifying this ensures consistent signatures
- Non-determinism only affects map types (not used in Receipt struct)

**2. Payer Address Source**
- Sourced from verifier's `RecoveredAddress` field
- Verifier already recovers wallet address from EIP-712 signature

**3. Receipt Lookup Authentication**
- Chose public lookup (no auth) approach
- Receipts act as public proof-of-payment (like blockchain transactions)
- Random receipt IDs prevent enumeration
- Only hashes stored, no sensitive full request/response data

**4. Storage Strategy**
- Implemented `ReceiptStore` interface for future extensibility
- Current: In-memory with automatic TTL cleanup
- Future: Easy swap to Redis/PostgreSQL via interface

### Integration Flow

```go
// In handleSummarize:
1. Verify payment (existing)
2. Call AI service (existing)
3. Generate cryptographic receipt (NEW)
   - Hash request and response bodies
   - Sign with server's private key
   - Store with TTL
4. Return receipt in header AND body
```

### Security Features

- ECDSA signatures using Keccak256 (Ethereum-compatible)
- Server private key loaded from environment
- Only hashes of request/response stored (not full content)
- Thread-safe storage with mutex
- Automatic expiration via TTL

## Test Results

```
ok      gateway 0.472s
```

**All 10 tests passing:**
- ✅ TestGenerateReceiptID - Format and uniqueness
- ✅ TestHashData - SHA-256 consistency
- ✅ TestSignReceipt - Receipt signing
- ✅ TestReceiptJSONSerialization - Deterministic output
- ✅ TestStoreAndRetrieveReceipt - Storage operations
- ✅ TestReceiptNotFound - 404 handling
- ✅ TestHashDataConsistency - Hash stability
- ✅ TestVerifyReceiptSignature - Signature verification

## API Examples

### Successful Request with Receipt

**Response**:
```json
{
  "result": "AI summary...",
  "receipt": {
    "receipt": {
      "id": "rcpt_a1b2c3d4e5f6",
      "version": "1.0",
      "timestamp": "2024-01-15T10:30:00Z",
      "payment": {
        "payer": "0x742d35Cc...",
        "recipient": "0x2cAF48b4...",
        "amount": "0.001",
        "token": "USDC",
        "chain_id": 8453,
        "nonce": "9c311e31-..."
      },
      "service": {
        "endpoint": "/api/ai/summarize",
        "request_hash": "sha256:abc123...",
        "response_hash": "sha256:def456..."
      }
    },
    "signature": "0x1234...",
    "server_public_key": "0xabcd..."
  }
}
```

**Headers**:
```
X-402-Receipt: <base64_encoded_receipt>
```

### Receipt Lookup

**Request**:
```bash
GET /api/receipts/rcpt_a1b2c3d4e5f6
```

**Response** (200):
```json
{
  "receipt": { ... },
  "signature": "0x...",
  "server_public_key": "0x...",
  "status": "valid"
}
```

**Response** (404):
```json
{
  "error": "Receipt not found",
  "message": "Receipt may have expired or never existed"
}
```

## Configuration

Add to `.env`:
```bash
# Required: Server's private key for signing receipts
SERVER_WALLET_PRIVATE_KEY=your_private_key_hex

# Optional: Receipt TTL (default 24 hours)
RECEIPT_TTL=86400
```

## Breaking Changes

None. This is purely additive - all existing endpoints continue to work unchanged.

## Future Enhancements

- [ ] TypeScript client verification utility
- [ ] Redis storage implementation using `ReceiptStore` interface
- [ ] PostgreSQL storage implementation
- [ ] Receipt search/query by payer, date range, amount
- [ ] Batch receipt verification
- [ ] Optional authentication for receipt lookups

## Checklist

- [x] Code follows project style guidelines
- [x] Tests added and passing (10/10 tests)
- [x] Documentation updated (.env.example)
- [x] Interface design for future extensibility (ReceiptStore)
- [x] No breaking changes
- [x] All acceptance criteria from #16 met
