# MicroAI Paygate Future Vision

## Product Thesis

MicroAI Paygate should become the local-first starter kit for AI API monetization.
The product promise is simple: an AI API or MCP tool builder can add paid HTTP
access locally, test the full challenge-sign-retry-receipt flow without secrets,
then graduate to official x402 and facilitator-backed settlement when they are
ready.

MicroAI Paygate is not trying to replace official x402, Coinbase CDP,
Cloudflare agent payments, or Stripe machine payments. It should be the
developer-friendly bridge into that ecosystem: easy to run, easy to inspect,
honest about what is local authorization versus real settlement, and structured
so the SDK can swap protocol adapters over time.

## Target Users

- AI API builders who want to charge per request without building payment
  plumbing first.
- MCP tool builders who need paid agent access to one or more tools.
- Hackathon and early-stage teams experimenting with x402-style agent payments.
- Developers who want a small reference stack for wallet signatures, HTTP 402
  challenges, receipts, and gateway-side enforcement.

## Product Promise

1. Run the whole paid API flow locally without OpenRouter, Ollama, Redis, or
   production secrets.
2. Use the TypeScript SDK to handle challenge parsing, EIP-712 signing, signed
   retries, receipt decoding, and trusted receipt verification.
3. Keep the current MicroAI local protocol for learning and demos.
4. Add official x402-v2 compatibility as a protocol adapter, not as a rewrite.
5. Add production hardening only after the local developer path is reliable.

## Roadmap

### Week 1: Credible Local Product

- Fix documentation that overclaims current Redis or settlement behavior.
- Add `AI_PROVIDER=mock` so local demos and CI do not need OpenRouter or Ollama.
- Make E2E run the real payment flow against the mock provider by default.
- Add a TypeScript SDK receipt verification example.

### Week 2: SDK As Product Surface

- Dogfood `sdk/typescript` inside `web/` so there is one client contract.
- Add a small external example app that imports `@microai/paygate-sdk`.
- Publish a "paid endpoint in 10 minutes" guide.

### Weeks 3-4: Official x402 Bridge

- Add an `x402-v2` SDK protocol adapter alongside the current MicroAI adapter.
- Add gateway configuration for `PAYMENT_PROTOCOL=microai-local|x402-v2`.
- Wire facilitator verification and settlement only when the official flow is
  tested end to end.

### Month 2: Production Trust

- Add Redis-backed verifier nonce storage before claiming multi-replica safety.
- Add structured request/payment logs.
- Add provider retries and circuit breaker behavior.
- Add Prometheus metrics and graceful shutdown.
- Keep body-size, timeout, CORS, and secret-handling docs aligned with code.

### Later: Market Proof

- Add a paid MCP tool example.
- Add streaming summaries once stream receipt semantics are explicit.
- Add multi-chain support after dynamic EIP-712 domain handling is stable.
- Add optional on-chain or facilitator-backed settlement verification.

## Success Metrics

- Three external repos run the mock local flow.
- Two external users complete a testnet paid request.
- One concrete user complaint changes the SDK API or documentation.

Stars are useful but not the main signal. The main signal is another developer
using the SDK in a real project and finding the first sharp edge.

## Non-Goals

- Do not claim real USDC settlement until settlement is implemented.
- Do not claim official x402 compatibility until the `x402-v2` adapter works
  against official-compatible flows.
- Do not claim multi-replica verifier safety until Redis-backed verifier nonce
  storage exists.
- Do not spend more time on landing-page polish until the local developer flow
  is deterministic.
