# MicroAI Paygate Tests

This directory contains End-to-End (E2E) tests for the MicroAI Paygate system.

## Prerequisites

- [Bun](https://bun.sh) installed
- The MicroAI Paygate stack running (`bun run stack` in the root directory)
 - Go toolchain (for gateway build in helper script)
 - Rust toolchain (for verifier build in helper script)

## Running E2E Tests

The E2E tests require the Gateway and Verifier services to be running. A helper script (`run_e2e.sh`) will build and start them for you before running tests.

```bash
bun run test:e2e
```

Or manually:

1. Start the stack:
   ```bash
   bun run stack
   ```

2. Run the tests:
   ```bash
   bun test tests/e2e.test.ts
   ```

Notes:
- If `OPENROUTER_API_KEY` is not set, signature verification will still pass, but the upstream AI call may return 500.
- The helper script expects ports 3000 and 3002 to be free; stop existing processes if needed.

## Other Tests

### Gateway (Go)
Unit tests for the Go Gateway are located in `gateway/`.
Run them with:
```bash
bun run test:go
# or: cd gateway && go test ./...
```

### Verifier (Rust)
Unit tests for the Rust Verifier are located in `verifier/src/main.rs`.
Run them with:
```bash
bun run test:rust
# or: cd verifier && cargo test
```

### All Unit Tests
```bash
bun run test:unit
```

> **Note:** Do NOT use `bun test` directly - it triggers bun's native test runner without starting services.
