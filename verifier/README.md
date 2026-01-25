# Verifier Service

The Verifier is a specialized microservice dedicated to cryptographic operations. Written in Rust, it provides a secure and isolated environment for validating EIP-712 signatures.

## Role & Responsibilities

- **Signature Validation**: Receives a payment context and a signature from the Gateway.
- **ECDSA Recovery**: Uses the `ethers-rs` library to recover the signer's address from the cryptographic signature.
- **Stateless Operation**: Performs pure computation without requiring database access or session state.

## Technology Stack

- **Language**: Rust (2021 Edition)
- **Web Framework**: Axum
- **Cryptography**: `ethers-rs` (bindings to `k256` and `secp256k1`)
- **Serialization**: Serde / Serde JSON

## Key Files

- `src/main.rs`: The single-file implementation containing the HTTP server and the `verify_signature` logic.
- `Cargo.toml`: Dependency definitions including `axum`, `tokio`, and `ethers`.
- `Dockerfile`: Multi-stage build configuration producing a minimal binary.

## Development

To run the verifier locally:

```bash
cargo run
```

The service listens on port 3002 by default.

## Configuration

Current implementation has no required env vars. It uses hardcoded EIP-712 domain values:

- `name`: MicroAI Paygate
- `version`: 1
- `chainId`: 1 (tests) / request payload (runtime)
- `verifyingContract`: 0x0000000000000000000000000000000000000000

If you change domain parameters in the gateway/frontend, update them here to stay in sync.

## API Endpoints

### Health Check

```bash
curl http://localhost:3002/health
```

**Response:**
```json
{
  "status": "healthy",
  "service": "verifier",
  "version": "<cargo pkg version>"
}
```

The health endpoint returns the service status, name, and current version from Cargo.toml. Use this endpoint to verify the verifier is running and to detect if the service is down.

### Signature Verification

```bash
curl -X POST http://localhost:3002/verify -H "Content-Type: application/json" -d '{"context":{...},"signature":"0x..."}'
```

## Testing

```bash
cargo test
```
