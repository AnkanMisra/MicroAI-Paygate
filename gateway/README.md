# Gateway Service

The Gateway is the high-performance entry point for the MicroAI Paygate architecture. Written in Go, it handles traffic orchestration, payment enforcement, and proxying to AI providers.

## Role & Responsibilities

- **Traffic Entry Point**: Listens on port 3000 and accepts all incoming API requests.
- **x402 Enforcement**: Inspects headers for `X-402-Signature`, `X-402-Nonce`, and `X-402-Timestamp`. If missing, it rejects the request with a 402 status and payment context.
- **Verification Orchestration**: Communicates with the internal Rust Verifier service to validate cryptographic signatures.
- **Proxying**: Forwards authenticated requests to the OpenRouter API and returns the response to the client.

## Technology Stack

- **Language**: Go (Golang) 1.24
- **Framework**: Gin Web Framework
- **Concurrency**: Goroutines for non-blocking I/O operations.

## Key Files

- `main.go`: Contains the server initialization, route definitions, and the core `handleSummarize` logic.
- `Dockerfile`: Multi-stage build configuration for creating a lightweight Alpine Linux container.

## Development

To run the gateway locally:

```bash
go run main.go
```

Ensure the Verifier service is running on port 3002 before starting the Gateway.

## Configuration

Environment variables (via `.env`):

**Required:**
- `OPENROUTER_API_KEY` — API key for OpenRouter (validated at startup)

**Optional:**
- `OPENROUTER_MODEL` — model name, default `z-ai/glm-4.5-air:free`
- `VERIFIER_URL` — override verifier endpoint, default `http://127.0.0.1:3002`
- `ALLOWED_ORIGINS` — comma-separated CORS allowed origins, default `http://localhost:3001`; values must be origins only, with no path/query/fragment
- `RECIPIENT_ADDRESS` — payment recipient; falls back to default if unset
- `CHAIN_ID` — chain id used in EIP-712 domain; default `84532` (Base Sepolia)
- `RECEIPT_STORE` — receipt storage backend, `redis` by default or `memory` for tests/local experiments
- `RECEIPT_TTL` — receipt TTL in seconds, default `86400`
- `REDIS_URL` — required when `CACHE_ENABLED=true` or `RECEIPT_STORE=redis`; use `redis:6379` in Compose and `localhost:6379` locally

**Rate Limiting:**
- `RATE_LIMIT_ENABLED` — enable/disable rate limiting (default: true)
- `RATE_LIMIT_ANONYMOUS_RPM` / `RATE_LIMIT_ANONYMOUS_BURST`
- `RATE_LIMIT_STANDARD_RPM` / `RATE_LIMIT_STANDARD_BURST`

**Request Timeouts:**
- `REQUEST_TIMEOUT_SECONDS` — global timeout (default: 60)
- `AI_REQUEST_TIMEOUT_SECONDS` — AI endpoint timeout (default: 30)
- `VERIFIER_TIMEOUT_SECONDS` — verifier timeout (default: 2)
- `HEALTH_CHECK_TIMEOUT_SECONDS` — health check timeout (default: 2)

Ports: Gateway listens on `3000` by default.

## Testing

```bash
go test ./...
```
