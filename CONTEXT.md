# MicroAI Paygate Context

## Domain Vocabulary

**Payment Context**
The gateway-issued x402 signing payload for one paid AI request. It contains the recipient, token, amount, chain ID, nonce, and timestamp that the client signs with EIP-712 typed data.

**Signed Retry**
The client request sent after a `402 Payment Required` challenge. It repeats the original AI request and includes `X-402-Signature`, `X-402-Nonce`, and `X-402-Timestamp` headers from the Payment Context.

**Verifier Result**
The Rust verifier's answer for a Signed Retry. A valid result includes the recovered wallet address; an invalid result includes a machine-readable `error_code` such as `invalid_signature`, `nonce_already_used`, or `chain_id_mismatch`.

**Paid Request**
A gateway request that has passed x402 verification and can either call the AI provider or serve an already-cached AI result. A Paid Request is the point where receipt generation becomes valid.

**Receipt**
The gateway-signed proof produced after a successful Paid Request. It records payment terms, payer, endpoint, request hash, response hash, and the gateway server public key.

**Receipt Store**
The persistence module for signed receipts. Redis is the default adapter; memory storage is used for local quick starts and tests.

**Response Cache**
The optional Redis-backed cache for AI summaries. A cache hit still requires a valid Signed Retry before the cached result can be returned.
