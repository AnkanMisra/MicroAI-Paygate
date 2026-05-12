# AGENTS.md - Instructions for AI Coding Agents

## Project Map
- **Gateway (`gateway/`)**: Go/Gin API gateway on port 3000. It owns request timeouts, CORS/gzip, token-bucket rate limits, Redis-backed response caching, x402 payment challenge/verification orchestration, OpenRouter/Ollama upstream calls, and receipt signing/storage.
- **Verifier (`verifier/`)**: Rust/Axum service on port 3002. It verifies EIP-712 payment signatures and returns recovered wallet information.
- **Web (`web/`)**: Next.js/Bun frontend on port 3001. It requests summaries, handles 402 payment contexts, prompts wallet chain switching/signing, retries with `X-402-*` headers, and displays results.
- **E2E (`tests/`, `run_e2e.sh`)**: Bun tests that exercise the gateway/verifier payment flow and may call the AI provider when `OPENROUTER_API_KEY` is available.
- **Docs/config**: `README.md`, service READMEs, `gateway/openapi.yaml`, `.env.example`, `docker-compose.yml`, `Makefile`, and `.github/workflows/*` must stay aligned with behavior and ports.

## Build & Test Commands
- **Full stack**: `bun run stack` (starts gateway:3000, web:3001, verifier:3002)
- **Gateway (Go)**: `cd gateway && go test -v ./...` | Single test: `go test -v -run TestName`
- **Verifier (Rust)**: `cd verifier && cargo test` | Single test: `cargo test test_name`
- **Web (Next.js)**: `cd web && bun run lint && bun run build`
- **E2E tests**: `bun run test:e2e` (requires `OPENROUTER_API_KEY` for a full upstream AI path)

## Lint & Format
- **Go**: `cd gateway && gofmt -w .` before committing Go changes; validate with `cd gateway && go vet ./...`
- **Rust**: `cd verifier && cargo fmt -- --check` and `cd verifier && cargo clippy -- -D warnings`
- **Web**: `cd web && bun run lint`
- **Type checks**: `cd web && bun run test` when TypeScript/frontend code changes.

## Code Style
- Follow language idioms: Go fmt/vet, Rust fmt/clippy, TypeScript ESLint.
- Use Bun (not npm/node) for JS/TS runtime commands in this repo.
- Commit prefixes: `feat:`, `fix:`, `docs:`, `chore:`.
- No secrets in code/logs; use `.env` files and placeholders in `.env.example`.
- Keep changes minimal and focused; add tests for new behavior.
- Ports: gateway=3000, web=3001, verifier=3002.
- Never put try/catch blocks around imports.

## Commit Attribution
- Every commit that Codex creates, amends, or explicitly drafts for this repo must include this exact trailer in the commit message:
  `Co-authored-by: codex <codex@users.noreply.github.com>`
- Keep the trailer in addition to any human or tool co-authors. Do not omit it from docs-only, review-followup, or small cleanup commits when Codex performed the work.

## Strict Codex Review Guidelines
When asked to review a PR, act like a senior engineer doing a pre-merge review. Inspect every changed file and enough surrounding code to trace the affected behavior. Do **not** stop at “no major issues” if there are concrete edge cases, missing tests, docs drift, CI gaps, or maintainability risks.

### Review Scope and Triage
- Verify the PR against the current code, not only the changed lines. Follow call paths across `gateway/`, `verifier/`, `web/`, tests, docs, workflows, and Docker/Compose files when they are affected.
- Verify the change matches the PR title, description, issue context, and surrounding behavior. For bug fixes, confirm the root cause is fixed; for refactors, confirm behavior is preserved; for features, check happy path, failure path, loading/empty states, and invalid input.
- Prioritize correctness, security, regressions, tests, performance, reliability, and maintainability. Explicitly consider null/undefined values, empty or invalid input, duplicates, async/race behavior, failed requests, missing permissions, and bad state.
- Prefer precise, fixable findings with file/line references and explain the user impact. Avoid vague style opinions unless they affect correctness, security, maintainability, or contributor experience. Do not flag normal async usage, formatter-managed formatting, or personal preference.
- Flag PR hygiene issues: broad/unrelated diffs, stale generated files, lockfile/package manifest mismatches, missing migration notes, broken badges/links, inconsistent service names, or unmentioned breaking changes.
- Confirm that docs and config changes stay evergreen. Move temporal project status notes out of the main README when possible and keep `.env.example`, service READMEs, OpenAPI, Docker, Makefile, and workflows synchronized. Treat typos and grammar mistakes in docs, comments, logs, errors, and UI text as reviewable user-facing defects.
- Check CI path filters whenever files move between services. A change to `gateway/**`, `verifier/**`, `web/**`, `tests/**`, dependency manifests/lockfiles, `run_e2e.sh`, Docker/Compose, or workflows should trigger the relevant safe checks.

### Gateway Review Checklist (`gateway/`)
- x402 flow: missing signatures should produce a complete 402 payment context; signed retries must bind signature, nonce, timestamp, recipient, amount, token, and chain ID consistently with the verifier and web client.
- Replay/expiry: look for nonce reuse bugs, receipt TTL mistakes, timestamp parsing errors, clock-skew regressions, cache keys that bypass payment verification, or receipt storage races.
- Security: ensure private keys/API keys are never logged, returned, committed, or added to OpenAPI examples. Validate CORS changes, header handling, request body limits, upstream error details, and user-controlled log output.
- Reliability: every outbound verifier/AI/Redis call should respect context deadlines and handle cancellation. Review timeout defaults and `REQUEST_TIMEOUT_SECONDS`, `AI_REQUEST_TIMEOUT_SECONDS`, `VERIFIER_TIMEOUT_SECONDS`, and health-check behavior.
- Rate limiting/cache: check per-IP/per-wallet bucket selection, cleanup, disabled-mode behavior, Redis URL validation, cache miss/fallback behavior, and concurrency safety.
- API contract: changes to routes, headers, status codes, receipts, or response bodies require matching tests and updates to `gateway/openapi.yaml`, README sections, and the web client if applicable.
- Tests: require table tests or targeted regression tests for new handlers, config parsing, middleware, timeout behavior, receipts, cache/rate limits, and error paths.

### Verifier Review Checklist (`verifier/`)
- EIP-712 parity: domain name/version, chain ID, verifying contract, field order/types, and message values must match the gateway, web frontend, and E2E tests.
- Cryptography correctness: reject malformed hex/signatures, wrong chain/recipient/token/amount, expired timestamps, invalid recovery IDs, and mismatched recovered addresses without panics.
- Axum behavior: validate request/response schemas, status codes, JSON errors, body-size limits, and timeout/DoS protections.
- Tests: add positive and negative cases for signature recovery, tampered fields, malformed input, expiration/skew boundaries, and compatibility with `ethers` signing.

### Web Review Checklist (`web/`)
- Wallet flow: verify MetaMask/EVM provider detection, chain switching/addition, signer refresh after network changes, rejected signature handling, and clear user-facing errors.
- Contract parity: payment context fields and EIP-712 types must match the gateway/verifier exactly; timestamps should remain numeric and nonce/header propagation must be complete.
- Next.js/React quality: keep client/server boundaries explicit (`"use client"` where needed), avoid leaking server-only env vars, preserve accessibility for forms/buttons/status messages, and prevent hydration or uncontrolled-input issues.
- Config: only `NEXT_PUBLIC_*` values may be read client-side. Gateway URL and chain IDs should stay documented in `web/README.md` and `.env.example`.
- Tests/checks: run lint/build/type checks for frontend changes and request UI screenshots for perceptible web UI changes.

### Cross-Service Review Checklist
- Protocol changes must update all participants: gateway structs, verifier structs, web typed data, E2E signing, OpenAPI, README diagrams/flow, and `.env.example` when env/config is involved.
- Dependency changes must be justified, minimal, and reflected in the correct lockfile (`bun.lock`, `web/bun.lock`, `web/package-lock.json`, `gateway/go.sum`, `verifier/Cargo.lock`).
- Docker/Compose changes must preserve service names, health expectations, env wiring, ports 3000/3001/3002, and local-vs-container hostnames (`localhost` vs service names such as `redis`/`verifier`).
- E2E changes must not require real secrets unless explicitly documented and safely skipped in CI. Mock/stub external providers where a deterministic unit test is enough.
- Documentation-only PRs still deserve review for stale commands, inconsistent versions, broken anchors, wrong ports, temporal claims, and mismatch with code/workflows.

### Expected Review Output
- Include findings for small but real issues: off-by-one TTL/expiry mistakes, missing negative tests, inconsistent env names, path filters that skip CI, docs that would mislead setup, and security footguns. Only flag real issues; if unsure, state the assumption clearly.
- Use these severities: `P0` critical security/data-loss/build/deploy failure; `P1` real bug, broken behavior, important edge case, bad error handling, or user-facing typo; `P2` maintainability, readability, missing tests, or moderate performance issue; `P3` optional cleanup or polish.
- Format each finding exactly as:
  - `[P0/P1/P2/P3] Short title`
  - `File: path`
  - `Problem: ...`
  - `Impact: ...`
  - `Fix: ...`
- If you find a bug, keep feedback constructive and include a brief encouraging note.
- End every review with `Verdict: approve`, `approve with comments`, or `request changes`, plus the biggest risk and the tests run or recommended.
- If no actionable issues remain after this checklist, say what areas were checked and why no findings were raised.
