# AGENTS.md - Instructions for AI Coding Agents

## Build & Test Commands
- **Full stack**: `bun run stack` (starts gateway:3000, web:3001, verifier:3002)
- **Gateway (Go)**: `cd gateway && go test -v ./...` | Single test: `go test -v -run TestName`
- **Verifier (Rust)**: `cd verifier && cargo test` | Single test: `cargo test test_name`
- **Web (Next.js)**: `cd web && bun run lint && bun run build`
- **E2E tests**: `bun run test:e2e` (requires OPENROUTER_API_KEY)

## Lint & Format
- **Go**: `go fmt ./...` and `go vet ./...`
- **Rust**: `cargo fmt -- --check` and `cargo clippy -- -D warnings`
- **Web**: `cd web && bun run lint`

## Code Style
- Follow language idioms: Go fmt/vet, Rust fmt/clippy, TypeScript ESLint
- Use Bun (not npm/node) for JS/TS runtime
- Commit prefixes: `feat:`, `fix:`, `docs:`, `chore:`
- No secrets in code/logs; use `.env` files
- Keep changes minimal and focused; add tests for new behavior
- Ports: gateway=3000, web=3001, verifier=3002
