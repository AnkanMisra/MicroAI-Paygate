# Changelog

## Unreleased

- **Breaking**: Add `timestamp` field (Unix seconds) to EIP-712 `Payment`/`PaymentContext` message used across verifier, gateway, and TypeScript clients.
- Add configurable signature expiry window via `SIGNATURE_EXPIRY_SECONDS` (default 300 seconds) and clock skew grace via `SIGNATURE_CLOCK_SKEW_SECONDS` (default 60 seconds).
- Rust verifier now validates timestamps and returns structured error codes:
  - `E007` for expired signatures
  - `E008` for future timestamps beyond allowed skew
  - `E009` for missing timestamp field
- Go gateway and TS client/web now populate and sign the `timestamp` field in payment contexts.
- Updated tests to cover timestamp edge cases (expired, future, boundary) and updated E2E flow to sign the new message shape.
