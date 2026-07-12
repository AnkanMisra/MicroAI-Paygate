# Security Policy

MicroAI Paygate handles wallet signatures, payment contexts, replay protection, receipts, Redis persistence, CORS, rate limits, and service-to-service calls. Please report vulnerabilities responsibly and do not publish exploit details before maintainers have had a reasonable chance to respond.

## Supported Scope

Security reports are welcome for the current `main` branch and the hosted deployment configuration documented in this repository.

In scope examples:

- EIP-712 signature verification bypass.
- Replay protection bypass or nonce reuse bugs.
- Timestamp expiry or clock-skew bypass.
- Chain ID, recipient, token, or amount mismatch that still verifies.
- Receipt signature or receipt lookup integrity bugs.
- Cache behavior that serves paid content without valid verification.
- CORS, trusted proxy, or rate-limit bypass with security impact.
- Secret exposure in code, logs, docs, workflows, Docker, or deployment files.
- Server-side request, dependency, or workflow issues that could compromise users, keys, funds, or infrastructure.

Out of scope examples:

- Reports that require a funded wallet intentionally configured by the reporter.
- Spam, social engineering, or denial-of-service against third-party providers.
- Generic dependency reports without an exploit path or project-specific impact.
- Issues that only affect local test keys or documented placeholder values.

## How To Report A Vulnerability

Do not open a public GitHub issue with vulnerability details.

Preferred path:

1. Use GitHub private vulnerability reporting from the repository security policy page when it is enabled:
   `https://github.com/AnkanMisra/MicroAI-Paygate/security/policy`
2. Include a clear description, affected component, impact, reproduction steps, and sanitized logs or requests.
3. Redact all private keys, API keys, Redis URLs, seed phrases, wallet secrets, and production identifiers.

Fallback path when private vulnerability reporting is not enabled:

1. Open a minimal public issue that says only: "I need to report a security issue privately."
2. Do not include exploit details, vulnerable code paths, secrets, screenshots, or proof-of-concept payloads in that public issue.
3. Wait for a maintainer to provide a private contact path.

## What To Include

- Affected component: gateway, verifier, web, E2E, Docker/Compose, deployment, workflow, or docs.
- Exact version or commit SHA tested.
- Environment details needed to reproduce.
- Reproduction steps or proof of concept, written so maintainers can validate safely.
- Expected result and actual result.
- Security impact: replay, impersonation, signature bypass, fund risk, secret exposure, denial of service, or privilege escalation.
- Suggested fix if you have one.

## Response Expectations

Maintainers aim to acknowledge valid private reports within 7 days. The fix timeline depends on severity, exploitability, and whether the issue affects hosted deployments, local development only, or documentation/configuration.

Please allow maintainers time to investigate, patch, test, and publish remediation notes before public disclosure.

## Safe Harbor

Good-faith research is welcome when it:

- Avoids privacy violations and data destruction.
- Avoids accessing, modifying, or exfiltrating data that is not yours.
- Avoids service disruption.
- Uses local development environments whenever possible.
- Reports findings privately and gives maintainers time to respond.

## Secret Handling

Never share:

- OpenRouter API keys.
- Server wallet private keys.
- Wallet seed phrases.
- Upstash Redis URLs or passwords.
- Full `.env` files.
- Unredacted `X-402-Signature` headers from real users.

Committed examples and benchmark fixtures use placeholders or deterministic local test keys. Do not fund test wallets.
