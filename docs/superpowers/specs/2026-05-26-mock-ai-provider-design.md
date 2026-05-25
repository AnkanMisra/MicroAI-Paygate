# Mock AI Provider Design

Date: 2026-05-26

## Summary

Add a deterministic gateway AI provider selected with `AI_PROVIDER=mock`. The provider makes local demos and CI reliable by removing the OpenRouter/Ollama network dependency while preserving the existing payment flow.

The mock provider only replaces the upstream AI call. It does not bypass wallet signing, verifier calls, nonce handling, receipt signing, receipt storage, or the custom `X-402-*` protocol.

## Goals

- Support `AI_PROVIDER=mock` in the gateway.
- Avoid requiring `OPENROUTER_API_KEY` when mock mode is selected.
- Return deterministic summary text for the same prompt.
- Respect request context cancellation and deadlines.
- Report healthy readiness for mock mode without making network calls.
- Document mock mode as a local demo and CI provider, not a production AI backend.

## Non-Goals

- No SDK changes.
- No web UI changes.
- No payment bypass mode.
- No fake verifier mode.
- No official x402 compatibility claim.
- No on-chain settlement changes.

## Architecture

The provider will live under `gateway/internal/ai/` and implement the existing `ai.Provider` interface:

```go
type Provider interface {
    Generate(ctx context.Context, prompt string) (string, error)
}
```

Provider selection remains centralized in the existing AI provider factory. `AI_PROVIDER=mock` will return a `MockProvider`; existing `openrouter` and `ollama` behavior remains unchanged.

Gateway configuration validation will treat `mock` like a local provider: it must not require `OPENROUTER_API_KEY`, but all payment and receipt configuration checks still apply.

## Behavior

`MockProvider.Generate(ctx, prompt)` will:

- Return immediately with `ctx.Err()` if the context is already canceled or expired.
- Produce stable summary text derived from the prompt.
- Avoid external network calls and secret reads.

Gateway request flow remains unchanged:

1. Unsigned request receives a `402` challenge.
2. Client signs the EIP-712 payment context.
3. Client retries with `X-402-Signature`, `X-402-Nonce`, and `X-402-Timestamp`.
4. Gateway verifies the payment with the verifier service.
5. Gateway calls the selected AI provider.
6. Gateway signs and returns the `X-402-Receipt` header.

## Readiness

`/readyz` will recognize `AI_PROVIDER=mock` and report it healthy without probing OpenRouter or Ollama. This keeps local demos deterministic while still exposing dependency health for real providers.

## Documentation

Update docs where provider setup is described:

- `.env.example`
- `README.md`
- `gateway/README.md`
- Test or contributor docs if they describe AI provider setup

Docs must state that mock mode is for local demos, interviews, and CI. They must not imply production AI quality, USDC movement, official x402 compatibility, or payment settlement.

## Test Plan

Implementation will use TDD:

1. Add a failing test that `AI_PROVIDER=mock` selects the mock provider.
2. Add a failing config validation test proving mock mode does not require `OPENROUTER_API_KEY`.
3. Add failing tests for deterministic mock output and context cancellation.
4. Add a failing readiness test proving mock mode reports healthy without external calls.
5. Implement the minimum gateway changes needed to pass each test.
6. Run the focused gateway tests, then the broader repo checks relevant to gateway changes.

## Risks

- Mock mode could be misunderstood as a payment bypass. Mitigation: keep payment flow unchanged and document the boundary clearly.
- Docs could overstate production readiness. Mitigation: use "deterministic local/demo provider" language and keep current payment honesty statements.
- Readiness could hide real provider failures if configured accidentally. Mitigation: only enable mock when `AI_PROVIDER=mock` is explicit.
