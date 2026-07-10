# Contributor Agent Guidelines

These guidelines apply to automated tools and coding agents used by external contributors.

## GitHub communication

- Keep issue and pull-request writing concise, relevant, and in the contributor's own voice.
- Do not add process commentary or generated-content notices to issue bodies, pull-request bodies, comments, reviews, inline replies, assignment notes, or closure messages.
- Do not mention the drafting tool unless a maintainer explicitly requests that information.
- Never post promotional, repetitive, or unrelated comments.

## Changes

- Read `README.md`, the relevant service README, and `CONTRIBUTING.md` before editing.
- Keep each pull request focused on one issue and avoid unrelated formatting changes.
- Add or update tests for behavioral changes.
- Keep cross-service contracts, documentation, configuration, and examples synchronized.
- Never commit secrets, credentials, private keys, or real API tokens.

## Validation

- Gateway: `cd gateway && go test -v ./... && go vet ./...`
- Verifier: `cd verifier && cargo fmt -- --check && cargo clippy -- -D warnings && cargo test`
- Web: `cd web && bun run lint && bun run test && bun run build`
- SDK: `cd sdk/typescript && bun run test && bun run typecheck`
- Use Bun for JavaScript and TypeScript commands in this repository.

Report which checks were run and disclose any checks that could not be completed.
