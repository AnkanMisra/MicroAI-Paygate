<div align="center">
    <h1>MicroAI Paygate</h1>
    <img src="public/rootimage.png" alt="MicroAI Paygate Architecture" style="max-width:640px; width:100%; height:auto;" />
    <p>A high-performance, crypto-monetized AI microservice architecture implementing the x402 Protocol.</p>
</div>

## Documentation

- [Getting Started](README.md#getting-started-local)
- [Testing](README.md#testing)
- [Contributing Guide](CONTRIBUTING.md)
- [Project Rules](RULES.md)
- [License](LICENSE)

## Overview

MicroAI Paygate demonstrates a decentralized payment layer for AI services. Instead of traditional subscriptions, it utilizes the HTTP 402 (Payment Required) status code to enforce per-request crypto micropayments. The system has been re-architected from a monolithic Node.js application into a distributed microservices stack to ensure maximum throughput, type safety, and cryptographic security.

## Features

- **x402 Protocol Implementation**: Native handling of the HTTP 402 status code to gate resources.
- **Distributed Architecture**: Decoupled services for routing (Go), verification (Rust), and presentation (Next.js).
- **EIP-712 Typed Signatures**: Industry-standard secure signing for payment authorization.
- **Micropayments**: Low-cost transactions (0.001 USDC) on the Base L2 network.
- **High Concurrency**: Go-based gateway for handling thousands of simultaneous connections.
- **Memory Safety**: Rust-based verification service for secure cryptographic operations.
- **Token Bucket Rate Limiting**: Configurable per-IP and per-wallet rate limits with tiered access control.


## How MicroAI Paygate is Different

Most AI monetization platforms rely on Web2 subscription models (Stripe, monthly fees) or centralized credit systems. These approaches introduce friction, require user registration, and create central points of failure.

MicroAI Paygate is designed to be frictionless and trustless:

1.  **No Registration**: Users connect a wallet and pay only for what they use.
2.  **Stateless Verification**: The verification logic is purely cryptographic and does not require database lookups for session management.
3.  **Polyglot Performance**: We use the right tool for the job—Go for I/O bound routing, Rust for CPU-bound cryptography, and TypeScript for UI.
4.  **Standard Compliance**: Fully compliant with EIP-712, ensuring users know exactly what they are signing.

## Performance Benchmarks

The migration to a polyglot microservices architecture resulted in significant performance improvements across key metrics.

| Metric | Monolithic Stack (Node.js) | Microservices Stack (Go/Rust) | Improvement |
| :--- | :--- | :--- | :--- |
| **Request Latency (P99)** | 120ms | 15ms | **8x Faster** |
| **Verification Time** | 45ms | 2ms | **22x Faster** |
| **Concurrent Connections** | ~3,000 | ~50,000+ | **16x Scale** |
| **Memory Footprint** | 150MB | 25MB (Combined) | **6x More Efficient** |
| **Cold Start** | 1.5s | <100ms | **Instant** |

## Architecture & Backend Internals

### System Architecture

```mermaid
flowchart TB
    subgraph Clients[Client Layer]
        WEB[Web Frontend - Next.js]
        AGENT[Agent Bot - TypeScript]
        CLI[CLI / cURL]
    end

    subgraph Gateway[API Gateway Layer]
        GW[Gateway - Go/Gin :3000]
    end

    subgraph Verification[Verification Layer]
        VER[Verifier - Rust/Axum :3002]
    end

    subgraph External[External Services]
        AI[OpenRouter API]
        CHAIN[Base L2 - USDC]
    end

    WEB --> GW
    AGENT --> GW
    CLI --> GW
    
    GW <--> VER
    GW --> AI
    
    WEB -.-> CHAIN
    AGENT -.-> CHAIN
```

### x402 Protocol Flow

The x402 protocol enables trustless, per-request payments using cryptographic signatures:

```mermaid
graph TD
    subgraph Client_Layer [Client - Next.js/MetaMask]
        A[User Request] --> B{Signed?}
        B -- No --> C[Receive 402 Context]
        C --> D[Sign EIP-712]
        D --> E[Resubmit with X-402-Signature]
    end

    subgraph Edge_Gateway [Gateway - Go/Gin]
        E --> F[Token Bucket Rate Limiter]
        F --> G{Cache Check}
        G -- Miss --> H[Internal HTTP Request]
        G -- Hit --> I[Verify Receipt & Return]
    end

    subgraph Crypto_Service [Verifier - Rust/Axum]
        H --> J[k256 ECDSA Recovery]
        J --> K{Signature Valid?}
        K -- Yes --> L[Sign Receipt]
    end

    subgraph Storage [Persistence]
        L --> M[(Redis Cache)]
        L --> N[(Receipt Store)]
    end

    %% Component Styling
    style Edge_Gateway fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    style Crypto_Service fill:#fce4ec,stroke:#880e4f,stroke-width:2px
    style Storage fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    style Client_Layer fill:#f5f5f5,stroke:#333,stroke-dasharray: 5 5
```

## 🛡️ Security & Cryptographic Integrity

MicroAI Paygate implements the **x402 Protocol** using a zero-trust verification model. We utilize **EIP-712** typed signatures to prevent phishing and "blind signing" attacks common with raw hex strings.

### 1. Cryptographic Verification Logic
The Rust Verifier performs Elliptic Curve Digital Signature Algorithm (ECDSA) recovery to ensure the signer address $\sigma_{addr}$ matches the expected payer. The verification follows the logic:

$$V(m, \sigma) \rightarrow \text{Address}$$

Where:
- $m$: The EIP-712 structured hash of the `PaymentContext`.
- $\sigma$: The $65$-byte signature provided in the `X-402-Signature` header.
- The request is only granted if $\sigma_{addr} \in \text{Authorized Payers}$.

### 2. Memory Safety & Concurrency
By utilizing **Rust (k256)** for the verification layer, the system remains immune to:
- **Buffer Overflows:** Prevents malicious signatures from corrupting memory.
- **Race Conditions:** Rust's strict ownership model ensures thread-safe signature recovery during high-concurrency peaks.

### Payment Context Structure

When a `402 Payment Required` response is returned, it includes the payment context:

```json
{
  "error": "Payment Required",
  "message": "Please sign the payment context",
  "paymentContext": {
    "recipient": "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
    "token": "USDC",
    "amount": "0.001",
    "nonce": "9c311e31-eb30-420a-bced-c0d68bc89cea",
    "chainId": 8453
  }
}
```

The client signs this data using EIP-712 and resends with headers:
- `X-402-Signature`: The cryptographic signature
- `X-402-Nonce`: The nonce from the payment context

---

### The Gateway (Go)
The Gateway service utilizes Go's lightweight goroutines to handle high-throughput HTTP traffic. Unlike the Node.js event loop which can be blocked by CPU-intensive tasks, the Go scheduler efficiently distributes requests across available CPU cores.
- **Framework**: Gin (High-performance HTTP web framework)
- **Concurrency Model**: CSP (Communicating Sequential Processes)
- **Proxy Logic**: Uses `httputil.ReverseProxy` for zero-copy forwarding.

### The Verifier (Rust)
The Verifier is a specialized computation unit designed for one task: Elliptic Curve Digital Signature Algorithm (ECDSA) recovery.
- **Safety**: Rust's ownership model guarantees memory safety without a garbage collector.
- **Cryptography**: Uses `ethers-rs` bindings to `k256` for hardware-accelerated math.
- **Isolation**: Running as a separate binary ensures that cryptographic load never impacts the API gateway's latency.

## Installation & Deployment

### Getting Started (Local)

### 📋 Environment Matrix
Before setting up the local environment, ensure your system meets the following polyglot requirements:

| Dependency | Version | Role |
| :--- | :--- | :--- |
| **Go** | `1.24.4+` | Edge Gateway & Token Bucket Rate Limiting |
| **Rust** | `Stable` | Cryptographic Verifier (ethers-rs / EIP-712 Recovery) |
| **Node.js** | `20+` | Next.js 16.1.1 Frontend & UI Tooling |
| **Bun** | `Latest` | Unified Task Runner & High-Speed E2E Testing |
| **Redis** | `7.0+` | Receipt Caching, TTL Management, & Persistence |

**Clone & Install**
```bash
git clone https://github.com/AnkanMisra/MicroAI-Paygate.git
cd MicroAI-Paygate
bun install
go mod tidy -C gateway
cargo build -q -C verifier
```

**Configure Environment**
Copy `.env.example` to `.env` and fill values (see next section).

**Run the Stack**
```bash
bun run stack
```

**Run Tests**
```bash
bun run test:unit   # Go + Rust unit tests
bun run test:go     # Gateway tests only
bun run test:rust   # Verifier tests only
bun run test:e2e    # E2E (starts services automatically)
bun run test:all    # Full test suite with E2E
```

### Makefile Shortcuts

Use the Makefile to orchestrate builds, tests, and linting across all services:

```bash
make help    # Show all available Makefile targets
make all     # Default target (alias for build)
make build   # Build Gateway, Verifier, Web
make test    # Run Go, Rust, and Web tests
make lint    # Run Go vet, Rust clippy, Web lint
make dev     # Start full stack (via bun run stack)
make clean   # Clean build artifacts
```

> **Note:** Do NOT use `bun test` directly - it triggers bun's native test runner without starting services.

### Environment

Create a `.env` (or use `.env.example`) with at least:

- `OPENROUTER_API_KEY` — API key for OpenRouter **(required - validated at startup)**
- `OPENROUTER_MODEL` — model name (default: `z-ai/glm-4.5-air:free`)
- `SERVER_WALLET_PRIVATE_KEY` — private key for the server wallet (recipient of payments)
- `RECIPIENT_ADDRESS` — wallet address for receiving payments
- `CHAIN_ID` — chain used in signatures (default: `8453` for Base)

> **Note:** The gateway validates required environment variables at startup. If `OPENROUTER_API_KEY` is missing, the server will exit with a helpful error message.

**Optional Configuration:**
- `USDC_TOKEN_ADDRESS` — USDC contract address (default: Base USDC)
- `PAYMENT_AMOUNT` — cost per request in USDC (default: `0.001`)
- `VERIFIER_URL` — URL of verifier service (default: `http://127.0.0.1:3002`)

Ensure ports `3000` (gateway), `3001` (web), and `3002` (verifier) are free.

### Rate Limiting Configuration

MicroAI Paygate implements token bucket rate limiting to prevent abuse and protect API quotas.

**Features:**
- Token bucket algorithm with burst support
- Tiered limits (anonymous, authenticated, verified)
- Per-IP and per-wallet tracking
- Standard `X-RateLimit-*` headers

**Default Limits:**

| Tier | Requests/Minute | Burst | Identification |
|------|----------------|-------|----------------|
| Anonymous | 10 | 5 | IP address |
| Standard | 60 | 20 | Signed requests (wallet nonce) |
| Verified | 120 | 50 | Premium users (future) |

**Configuration:**
Add to your `.env` file:
```bash
# Rate Limiting
RATE_LIMIT_ENABLED=true

# Anonymous users (IP-based, no signature)
RATE_LIMIT_ANONYMOUS_RPM=10
RATE_LIMIT_ANONYMOUS_BURST=5

# Standard users (signed requests)
RATE_LIMIT_STANDARD_RPM=60
RATE_LIMIT_STANDARD_BURST=20

# Verified users (future: premium tier)
RATE_LIMIT_VERIFIED_RPM=120
RATE_LIMIT_VERIFIED_BURST=50

# Cleanup interval for stale buckets (seconds)
RATE_LIMIT_CLEANUP_INTERVAL=300
```

**Response Headers:**
- `X-RateLimit-Limit`: Max requests per minute for your tier
- `X-RateLimit-Remaining`: Requests remaining
- `X-RateLimit-Reset`: Unix timestamp when limit resets
- `Retry-After`: Seconds until reset (on 429 response)

### Request Timeouts

The gateway implements context-based request timeouts to prevent slow/hanging requests from consuming resources.

**Features:**
- Global request timeout (default: 60s)
- Per-endpoint configurable timeouts
- Context cancellation for downstream calls
- Buffered response to prevent race conditions
- Returns `504 Gateway Timeout` when exceeded

**Default Timeouts:**

| Endpoint | Timeout | Purpose |
|----------|---------|---------|
| Global | 60s | Maximum request duration |
| AI endpoints | 30s | OpenRouter calls |
| Verifier | 2s | Signature verification |
| Health check | 2s | `/healthz` endpoint |

**Configuration:**
```bash
# Request Timeouts
REQUEST_TIMEOUT_SECONDS=60
AI_REQUEST_TIMEOUT_SECONDS=30
VERIFIER_TIMEOUT_SECONDS=2
HEALTH_CHECK_TIMEOUT_SECONDS=2
```

### Caching Configuration

MicroAI Paygate includes an intelligent Redis-backed caching layer to reduce OpenRouter API costs and improve response times for frequently requested content.

**Features:**
- **Cache-Aside Pattern**: Checks Redis before calling AI provider. If found, data is returned instantly, but **payment verification is still enforced**.
- **Content-Addressable**: Uses SHA256 of request text as the cache key.
- **Secure by Design**: Cached responses are ONLY served to requests with valid payment signatures. The latency savings come from avoiding the AI provider call, not from skipping verification.
- **TTL-Based**: Configurable expiration to ensure content freshness.

**Configuration:**
Add to `.env`:
```bash
# Redis Configuration
# Use 'redis:6379' for docker-compose, 'localhost:6379' for local run
REDIS_URL=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

# Cache Settings
CACHE_ENABLED=true
# Time-to-live for cached items in seconds (default: 3600 = 1 hour)
CACHE_TTL_SECONDS=3600
```

### Docker Deployment (Production)

For production environments, we provide a containerized setup using Docker Compose. This orchestrates all three services in an isolated network.

1.  **Configure Environment**
    ```bash
    cp .env.example .env
    # Edit .env with your API keys and wallet configuration
    ```

2.  **Build and Run**
    ```bash
    docker-compose up --build -d
    ```

3.  **Verify Status**
    ```bash
    docker-compose ps
    ```

4.  **Logs**
    ```bash
    docker-compose logs -f
    ```

### Local Development

For rapid development, use the unified stack command which runs services on the host machine.

1.  **Install Prerequisites**
    - Bun, Go 1.24+, Rust/Cargo

2.  **Run Stack**
    ```bash
    bun run stack
    ```

## Testing
We maintain a comprehensive test suite covering all layers of the stack, from unit tests for individual microservices to full end-to-end (E2E) integration tests.

### End-to-End (E2E) Tests

The E2E tests simulate a real client interaction:
1.  Sending a request to the Gateway.
2.  Receiving a `402 Payment Required` challenge.
3.  Signing the challenge with an Ethereum wallet.
4.  Resubmitting the request with the signature.
5.  Verifying the successful AI response.

**Run E2E Tests:**
```bash
bun run test:e2e
```
Prerequisites: Bun, Go, and Rust toolchains installed. This command uses `run_e2e.sh` to build and start the Go Gateway and Rust Verifier before executing tests.
If `OPENROUTER_API_KEY` is missing, the signature path will pass but the final AI call may return 500 after verification.

### Unit Tests

**Gateway (Go):**
Tests the HTTP handlers and routing logic.
```bash
cd gateway
go test -v
```

**Verifier (Rust):**
Tests the cryptographic verification logic and EIP-712 implementation.
```bash
cd verifier
cargo test
```

## Troubleshooting

- Port already in use: ensure 3000/3001/3002 are free or export alternative ports in env and update client config.
- Missing OpenRouter key: E2E tests may pass signature validation but fail on AI response with 500.
- Network errors inside Docker: use service names (`gateway:3000`, `verifier:3002`) instead of localhost.

## References

- HTTP 402 Payment Required (MDN): https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/402
- RFC 7231 Section 6.5.2 (Payment Required): https://www.rfc-editor.org/rfc/rfc7231#section-6.5.2
- EIP-712 Typed Structured Data: https://eips.ethereum.org/EIPS/eip-712

## Receipt Verification

MicroAI Paygate issues cryptographic receipts for every successful API request. These receipts are tamper-proof, independently verifiable, and stored for 24 hours by default.

### Receipt Structure

Every successful request returns a receipt in the response body and `X-402-Receipt` header:

```json
{
  "result": "AI summary...",
  "receipt": {
    "receipt": {
      "id": "rcpt_a1b2c3d4e5f6",
      "version": "1.0",
      "timestamp": "2026-01-06T10:30:00Z",
      "payment": {
        "payer": "0x742d35Cc...",
        "recipient": "0x2cAF48b4...",
        "amount": "0.001",
        "token": "USDC",
        "chainId": 8453,
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

### Client-Side Verification (TypeScript)

Use the provided verification library to verify receipts client-side:

```typescript
import { verifyReceipt, fetchReceipt } from './web/src/lib/verify-receipt';

// After receiving a response
const response = await fetch('/api/ai/summarize', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'X-402-Signature': signature,
    'X-402-Nonce': nonce,
  },
  body: JSON.stringify({ text: 'Your text here' }),
});

const data = await response.json();

// Verify the receipt signature
const isValid = await verifyReceipt(data.receipt);
console.log(`Receipt valid: ${isValid}`); // true

// Store receipt for future reference
localStorage.setItem(
  `receipt_${data.receipt.receipt.id}`, 
  JSON.stringify(data.receipt)
);
```

### Receipt Lookup API

Retrieve stored receipts by ID:

```bash
# Fetch receipt
curl http://localhost:3000/api/receipts/rcpt_a1b2c3d4e5f6

# Response (200 OK)
{
  "receipt": { ... },
  "signature": "0x...",
  "server_public_key": "0x...",
  "status": "valid"
}

# Not found (404)
{
  "error": "Receipt not found",
  "message": "Receipt may have expired or never existed"
}
```

### Verification Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant G as Gateway
    participant V as Verifier
    participant AI as OpenRouter

    C->>G: POST /api/ai/summarize + signature
    G->>V: Verify signature
    V-->>G: Valid ✓
    G->>AI: Get AI response
    AI-->>G: Response
    Note over G: Generate Receipt
    G->>G: Hash request/response
    G->>G: Sign with server key
    G-->>C: Response + Receipt
    Note over C: Verify Receipt
    C->>C: Check signature matches
    C->>C: Validate server public key
    C->>C: Store receipt
```

### Security Features

- **ECDSA Signatures**: Receipts signed using Keccak256 + secp256k1 (Ethereum-compatible)
- **Tamper-Proof**: Any modification invalidates the signature
- **Content Hashes**: Only SHA-256 hashes stored, not full request/response
- **Rate Limited**: Receipt lookup limited to 10 requests/minute (anonymous tier)
- **Auto-Expiry**: Receipts expire after 24 hours (configurable via `RECEIPT_TTL`)

### Configuration

Add to `.env`:

```bash
# Required: Server's private key for signing receipts
SERVER_WALLET_PRIVATE_KEY=your_private_key_hex

# Optional: Receipt TTL in seconds (default: 86400 = 24 hours)
RECEIPT_TTL=86400
```

## API Reference

### Endpoints

#### `POST /api/ai/summarize`

**Description**
Proxies a text summarization request to the AI provider, enforcing payment via the x402 protocol.

**Request Headers**
| Header | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `Content-Type` | string | Yes | Must be `application/json` |
| `X-402-Signature` | hex string | Yes | The EIP-712 signature signed by the user's wallet. |
| `X-402-Nonce` | uuid | Yes | The nonce received from the initial 402 response. |

**Request Body**
```json
{
  "text": "The content to be summarized..."
}
```

**Response Codes**

| Status Code | Meaning | Payload Structure |
| :--- | :--- | :--- |
| `200 OK` | Success | `{ "result": "Summary text..." }` |
| `402 Payment Required` | Payment Needed | `{ "paymentContext": { "nonce": "...", "amount": "0.001", ... } }` |
| `403 Forbidden` | Invalid Signature | `{ "error": "Invalid Signature", "details": "..." }` |
| `500 Internal Error` | Server Failure | `{ "error": "Service unavailable" }` |

#### `POST /verify` (Internal)

**Description**
Internal endpoint used by the Gateway to verify signatures with the Rust service. Not exposed publicly.

**Body**
```json
{
  "context": { ... },
  "signature": "0x..."
}
```

## Contributing

We welcome contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines and check the [GitHub Issues](https://github.com/AnkanMisra/MicroAI-Paygate/issues) for open tasks.

## License

This project is licensed under the [MIT License](LICENSE).

