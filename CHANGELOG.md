# Changelog

## Unreleased

- Refresh public contributor documentation for the current Go gateway, Rust verifier, Next.js web, Redis receipt/cache, and Render/Vercel/Upstash deployment-prep architecture.
- Add the private `@microai/paygate-sdk` TypeScript package for 402 challenge handling, EIP-712 signing, signed retries, receipt decoding, and trusted-key receipt verification.
- Add the MDX-backed website docs experience under `/docs`, covering quickstart, SDK usage, API, protocol, architecture, operations, and security limits.
- Add open source community files: support guide, code of conduct, issue templates, and pull request template.
- Expand `gateway/openapi.yaml` to document readiness and receipt lookup endpoints alongside the x402 summarize flow.
- Align local setup docs, service READMEs, environment examples, and contribution checks with the current codebase.
- Add configurable gateway CORS origins via `ALLOWED_ORIGINS`.
- Add configurable receipt storage via `RECEIPT_STORE`, defaulting to Redis-backed receipts with `memory` available for tests and local experiments.
- Add a reproducible verifier micro-benchmark harness under `bench/` with raw `wrk` output captured in `bench/RESULTS-2026-05-13.txt`.
- Remove unsupported benchmark multiplier claims from the README.
- **Breaking**: Add `timestamp` field (Unix seconds) to EIP-712 `Payment`/`PaymentContext` message used across verifier, gateway, and TypeScript clients.
- Add configurable signature expiry window via `SIGNATURE_EXPIRY_SECONDS` (default 300 seconds) and clock skew grace via `SIGNATURE_CLOCK_SKEW_SECONDS` (default 60 seconds).
- Rust verifier now validates timestamps and returns structured error codes:
  - `E007` for expired signatures
  - `E008` for future timestamps beyond allowed skew
  - `E009` for missing timestamp field
- Go gateway and TS client/web now populate and sign the `timestamp` field in payment contexts.
- Updated tests to cover timestamp edge cases (expired, future, boundary) and updated E2E flow to sign the new message shape.
