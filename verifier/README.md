# Verifier Service

The verifier is a Rust/Axum service on port `3002`. It validates EIP-712 payment signatures for the gateway and rejects malformed signatures, wrong-chain contexts, expired/future timestamps, and replayed nonces for a single verifier instance.

## Responsibilities

- Accept `POST /verify` requests from the gateway.
- Enforce EIP-712 domain parity with gateway, web, and E2E signing code.
- Recover the signer address from the wallet signature.
- Reject chain ID mismatches before signature acceptance.
- Reject expired timestamps and future timestamps beyond allowed clock skew.
- Reject reused nonce hashes inside the configured signature window.
- Return structured `error_code` values that the gateway maps to sanitized public errors.

## EIP-712 Domain

| Field | Value |
| --- | --- |
| `name` | `MicroAI Paygate` |
| `version` | `1` |
| `chainId` | `EXPECTED_CHAIN_ID`, falling back to `CHAIN_ID`, then `84532` |
| `verifyingContract` | `0x0000000000000000000000000000000000000000` |

Payment type:

```text
Payment(
  address recipient,
  string token,
  string amount,
  string nonce,
  uint256 timestamp
)
```

Any change to this shape must be applied together in:

- `gateway/main.go`
- `verifier/src/main.rs`
- `web/src/lib/x402-client.ts`
- `sdk/typescript/src/payment.ts`
- `tests/e2e.test.ts`
- `gateway/openapi.yaml`
- Root and service documentation

## API

### `GET /health`

Returns:

```json
{
  "status": "healthy",
  "service": "verifier",
  "version": "<cargo package version>"
}
```

### `POST /verify`

Request shape:

```json
{
  "context": {
    "recipient": "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
    "token": "USDC",
    "amount": "0.001",
    "nonce": "550e8400-e29b-41d4-a716-446655440000",
    "chainId": 84532,
    "timestamp": 1700000000
  },
  "signature": "0x..."
}
```

Successful response:

```json
{
  "is_valid": true,
  "recovered_address": "0x...",
  "error": null
}
```

Business rejection response:

```json
{
  "is_valid": false,
  "recovered_address": null,
  "error": "human-readable verifier detail",
  "error_code": "invalid_signature"
}
```

Important error codes:

| Code | Meaning |
| --- | --- |
| `invalid_signature` | Signature recovery failed or signer did not match the context. |
| `chain_id_mismatch` | Payment context chain does not match verifier expectation. |
| `timestamp_expired` | Timestamp is older than `SIGNATURE_EXPIRY_SECONDS`. |
| `timestamp_future` | Timestamp is beyond allowed future skew. |
| `timestamp_missing` | Timestamp field is missing or invalid. |
| `nonce_already_used` | Nonce hash was already accepted inside the signature window. |

## Configuration

| Variable | Default | Notes |
| --- | --- | --- |
| `MAX_REQUEST_BODY_BYTES` | `1048576` | JSON body size limit. |
| `EXPECTED_CHAIN_ID` | `84532` | Preferred chain ID enforcement variable. |
| `CHAIN_ID` | unset | Fallback when `EXPECTED_CHAIN_ID` is unset. |
| `SIGNATURE_EXPIRY_SECONDS` | `300` | Signature freshness window and nonce retention TTL. |
| `SIGNATURE_CLOCK_SKEW_SECONDS` | `60` | Allowed future timestamp skew. |

## Replay Protection

Nonce replay protection is in memory. It protects one verifier process. Production multi-replica verifier deployments need a shared nonce store such as Redis so every replica rejects the same replayed nonce.

Keep the verifier to one service instance or replica until shared nonce storage exists.

## Local Development

```bash
cd verifier
cargo run
```

The service listens on `0.0.0.0:3002` by default.

## Testing

```bash
cd verifier
cargo fmt -- --check
cargo clippy -- -D warnings
cargo test
```

Run these checks after changing EIP-712 fields, chain ID parsing, timestamp logic, nonce replay protection, request body limits, response schemas, or dependencies.
