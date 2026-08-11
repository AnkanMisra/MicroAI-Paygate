# E2E Tests

The `tests/` directory contains Bun end-to-end coverage for the gateway and verifier payment flow.

## What The E2E Flow Covers

- Unsigned `POST /api/ai/summarize` returns `402 Payment Required`.
- The 402 response includes a v2 context bound to the audience, method, resource, content type, and exact body hash.
- A test wallet signs the request-bound context and claimed payer with EIP-712 typed data.
- The signed retry includes `X-402-Signature`, `X-402-Nonce`, `X-402-Timestamp`, and `X-402-Payer`.
- The signed request is accepted by the verifier and proceeds to the AI provider.
- Changing one request byte after signing returns `403 signer_mismatch`.
- Reusing the same signed context returns `409 nonce_already_used`.

## Prerequisites

- Bun
- Go toolchain
- Rust toolchain
- Ports `3000` and `3002` free, plus `3100` when `OPENROUTER_API_KEY` is unset
- No external AI key is required; the helper starts a deterministic OpenRouter-compatible mock when `OPENROUTER_API_KEY` is unset.

When these variables are unset, the helper defaults the gateway to:

- `RECEIPT_STORE=memory`
- `CACHE_ENABLED=false`

The verifier still honors `VERIFIER_NONCE_STORE` from the environment. If you copied `.env.example`, override its Redis mode when running E2E without Redis.

## Run

From the repository root:

```bash
RECEIPT_STORE=memory \
VERIFIER_NONCE_STORE=memory \
CACHE_ENABLED=false \
bun run test:e2e
```

This runs `run_e2e.sh`, which builds and starts the verifier and gateway before executing:

```bash
bun test tests/e2e.test.ts
```

Do not use plain `bun test` as a replacement unless you have already started the required services yourself.

## Manual Flow

In one shell:

```bash
RECEIPT_STORE=memory VERIFIER_NONCE_STORE=memory CACHE_ENABLED=false bun run stack
```

In another shell:

```bash
bun test tests/e2e.test.ts
```

## SDK Tests

The SDK tests live in `sdk/typescript/src/__tests__/` and do not require the gateway or verifier to be running:

```bash
bun run test:sdk
```

They cover EIP-712 signing parity, the exact `X-402-*` signed retry headers, `X-402-Receipt` decoding, gateway-format receipt verification against a trusted signing key, and the mocked unsigned request -> `402` challenge -> signed retry flow.

The optional live SDK test is skipped by default. It assumes `bun run stack` is already running:

```bash
cd sdk/typescript
PAYGATE_SDK_LIVE_TEST=1 EVM_PRIVATE_KEY=0x... PAYGATE_SERVER_PUBLIC_KEY=0x... PAYGATE_GATEWAY_URL=http://localhost:3000 bun test src/__tests__/live-gateway.test.ts
```

Use only unfunded local or test wallet keys for live SDK tests. `PAYGATE_SERVER_PUBLIC_KEY` must be the gateway receipt signing public key distributed out of band; the SDK does not trust the key embedded in a receipt by itself.

## Reading Failures

When a live `OPENROUTER_API_KEY` is supplied, the signed request may return `502 upstream_unavailable` or `504 upstream_timeout` if OpenRouter is unavailable or slow. The default deterministic mock path must return `200`.

Failures that usually indicate payment-flow regressions:

- Initial request is not `402`.
- Payment context lacks v2 request-binding fields, `nonce`, `chainId`, or `timestamp`.
- Signed retry returns `400 invalid_timestamp`.
- Signed retry returns `403 invalid_signature`.
- A changed request does not return `403 signer_mismatch`.
- Replay does not return `409 nonce_already_used`.
