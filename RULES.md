# Project Rules

These guidelines keep the project maintainable and secure.

## Code
- Keep changes focused and minimal; avoid drive-by refactors.
- Add or update tests with behavior changes.
- Follow language idioms (Go fmt/vet; Rust fmt/clippy; TS linting where applicable).
- No secrets in code or logs. Use `.env` and `.gitignore` appropriately.

## Git & Reviews
- Prefer small, reviewable PRs.
- Write meaningful commits (`feat:`, `fix:`, `docs:`, `chore:`).
- Every commit created, amended, or explicitly drafted by Codex must include `Co-authored-by: codex <codex@users.noreply.github.com>`.
- Keep branches up to date with `main`/`ops` before requesting review.
- Do not force-push shared branches without coordination.

## Codex / AI Review Expectations
- Codex reviews must be strict, senior-engineer, specific, and actionable; do not limit feedback to only major bugs.
- Review the whole affected flow across gateway, verifier, web, tests, docs, CI, Docker/Compose, and dependency manifests when a PR touches cross-service behavior.
- Verify the PR matches its title/description and preserves expected behavior; for bug fixes, confirm the root cause is fixed.
- Call out small correctness and maintainability issues such as missing negative tests, edge cases, stale docs, stale OpenAPI examples, wrong CI path filters, package/lockfile drift, unclear PR descriptions, and broken local setup commands.
- Treat x402 payment verification, EIP-712 field parity, nonce/timestamp replay protection, receipt/cache behavior, rate limits, request timeouts, CORS, and secret handling as security-sensitive review areas.
- Require docs/config updates for changed env vars, ports, headers, status codes, API responses, Docker service names, or frontend wallet behavior.
- Prefer concise findings with severity (`P0`-`P3`), file/line references, user impact, and the smallest safe fix; skip purely subjective style comments unless they affect reliability or maintainability.
- End reviews with a verdict, biggest risk, and tests run or recommended.

## Security
- Never commit private keys or API tokens.
- Report vulnerabilities privately (see CONTRIBUTING for disclosure guidance).
- Keep dependencies minimal; avoid adding heavy libraries without justification.

## Documentation
- Update README/Service READMEs when behavior, env vars, or APIs change.
- Document new endpoints and flags in the relevant service README.
- Note breaking changes clearly in PR descriptions.

## Testing
- Run relevant test suites before PRs: gateway `go test`, verifier `cargo test`, E2E `bun run test:e2e` when applicable.
- Avoid merging code with failing tests unless marked and explained.

## Operations
- Default ports: 3000 (gateway), 3001 (web), 3002 (verifier). Avoid collisions or configure overrides.
- Docker usage: prefer service names (gateway/verifier/web) inside Compose.
- Log responsibly; avoid sensitive data in logs.
