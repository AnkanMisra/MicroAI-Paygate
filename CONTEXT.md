# MicroAI Paygate Context

## Glossary

### Payment Context

The challenge data returned with HTTP `402 Payment Required`. It contains the
recipient, token, amount, nonce, chain ID, and timestamp that the client signs.

### Signed Retry

A retry of the original paid request with wallet signature headers attached.
In the current local protocol, those headers are `X-402-Signature`,
`X-402-Nonce`, and `X-402-Timestamp`.

### Paid Request

A request that has passed signature verification and is allowed to reach the AI
provider.

### Receipt

A gateway-signed record binding a verified payment context to the request and
response hashes. It proves what the gateway signed, not that money moved
on-chain.

### Receipt Store

The storage used to look up signed receipts until their TTL expires.

### Settlement

The movement or verification of actual payment value. The current local
protocol does not perform settlement.

### Facilitator

An external service in official x402-style flows that verifies and settles
payment payloads.

### Mock Provider

A deterministic AI provider adapter used for local demos and CI. It exercises
the payment flow without a live model provider.

### SDK Protocol Adapter

The SDK module that knows how to parse a payment challenge, sign it, build retry
headers, and decode receipts for one wire protocol.
