# Security Policy

MicroAI Paygate is a decentralized payment gateway for AI services that enforces per-request crypto micropayments using HTTP 402 (Payment Required).  
Given its use of cryptography, distributed microservices, and on-chain interactions, security is a top priority.

---

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly.

🚫 **Do NOT open a public GitHub issue for security vulnerabilities.**  
Public disclosure may put users, wallets, or funds at risk.

### How to Report

Please report security issues privately via one of the following:
- GitHub private messages to the repository maintainers, or
- Any official security contact listed in the repository (if available)

When reporting, please include:
- A detailed description of the vulnerability
- Steps to reproduce the issue (proof-of-concept if possible)
- Affected component or service (Gateway, Verifier, UI, etc.)
- Potential impact (fund loss, replay attack, DoS, auth bypass, etc.)
- Relevant logs, traces, or request samples (redacted where appropriate)

---

## Security Scope

Security issues may include, but are not limited to:

### Payment & Cryptography
- EIP-712 signature verification issues
- Replay attacks or nonce misuse
- Incorrect payment validation
- Wallet impersonation or authorization bypass
- Chain/network mismatch (Base L2)

### Protocol & Networking
- Incorrect HTTP 402 handling
- Request tampering or downgrade attacks
- Rate-limit bypass (per-IP or per-wallet)
- Abuse of token bucket configuration

### Microservices Architecture
- Insecure service-to-service communication
- Authentication or authorization flaws between services
- Memory safety issues (especially in non-Rust components)
- Race conditions under high concurrency

### Web & API
- API authentication issues
- Injection vulnerabilities
- Exposure of secrets or private keys
- Misconfigured environment variables

---

## Responsible Disclosure

Please allow maintainers a reasonable amount of time to:
- Investigate the issue
- Develop and deploy a fix
- Notify users if necessary

We kindly ask that vulnerabilities are not disclosed publicly until a fix has been released.

---

## Security Design Notes

MicroAI Paygate incorporates multiple defense layers:
- Typed cryptographic signatures (EIP-712)
- Rust-based verification for memory safety
- Go-based high-concurrency gateway
- Strict rate-limiting per IP and wallet
- Minimal on-chain payment amounts to reduce exposure

---

Thank you for helping keep MicroAI Paygate secure and reliable.
