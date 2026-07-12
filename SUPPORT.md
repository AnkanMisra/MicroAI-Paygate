# Support

Use the right channel so maintainers can respond quickly and safely.

## Questions And Setup Help

Open a GitHub issue with the `question` label when you need help understanding the architecture, local setup, tests, Docker Compose, or deployment prep. Include:

- Your operating system and tool versions.
- The command you ran.
- The exact error output, with secrets removed.
- Which service you were working on: gateway, verifier, web, tests, bench, deployment, or docs.

## Bug Reports

Use the bug report issue template. Include reproduction steps, expected behavior, actual behavior, logs, and the validation commands you already ran.

## Feature Requests

Use the feature request issue template. Explain the use case, affected component, expected behavior, and whether the change affects x402 payment flow, EIP-712 signing, receipts, environment variables, Docker, deployment, or public API docs.

## Security Reports

Do not open a public issue for vulnerabilities, leaked secrets, private keys, replay bypasses, signature validation bugs, wallet impersonation, or anything that could put funds or users at risk.

Follow [SECURITY.md](SECURITY.md) for private vulnerability reporting. If private vulnerability reporting is not enabled on GitHub, open a minimal public issue that says only: "I need to report a security issue privately." Do not include exploit details in that issue.

## What Not To Post

- OpenRouter API keys.
- Server wallet private keys.
- Wallet seed phrases.
- Upstash Redis URLs or passwords.
- Full `.env` files.
- Unredacted request headers containing payment signatures.
