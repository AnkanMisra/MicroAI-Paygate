# Web Frontend

The web app is a Next.js/Bun frontend on port `3001`. It lets users submit text for summarization, receives `402 Payment Required` contexts from the gateway, prompts an EVM wallet to switch chain and sign EIP-712 typed data, and retries the request with x402 headers.

## Responsibilities

- Send unsigned summarize requests to the gateway.
- Detect `402` responses and read `paymentContext`.
- Detect an injected EVM provider such as MetaMask, Rabby, or Coinbase Wallet.
- Switch or add the requested chain when the wallet is on the wrong network.
- Sign the gateway-provided EIP-712 payment context.
- Retry with `X-402-Signature`, `X-402-Nonce`, and `X-402-Timestamp`.
- Display summary results or user-facing errors.

## Current Configuration

The frontend reads these `NEXT_PUBLIC_*` environment variables at build time:

| Variable | Default | Notes |
| --- | --- | --- |
| `NEXT_PUBLIC_GATEWAY_URL` | `http://localhost:3000` | Gateway base URL the browser fetches `/api/ai/summarize` and `/api/receipts/:id` from. |
| `NEXT_PUBLIC_EXPECTED_CHAIN_ID` | `84532` | Chain id the wallet widget expects. Must match the gateway's `CHAIN_ID`. Deployments on Base mainnet should set `8453` so the widget doesn't fight every payment context. |
| `NEXT_PUBLIC_EXPECTED_CHAIN_NAME` | `Base Sepolia` | Display name used by the wallet widget's `Switch to <name>` button and the summarize form's placeholder copy. |
| `NEXT_PUBLIC_PAYMENT_AMOUNT` | `0.001` | Pre-challenge fee label shown under the summarize form. **Informational only** — the actual signed amount is whatever the gateway embeds in the 402 payment context. |
| `NEXT_PUBLIC_PAYMENT_TOKEN` | `USDC` | Token symbol shown next to `NEXT_PUBLIC_PAYMENT_AMOUNT`. Same caveat — display-only. |

The signed amount, recipient, chain id, nonce, and timestamp at signing time **always come from the gateway's payment context**, not from these vars. The `NEXT_PUBLIC_*` values are display defaults so deployers running with non-default `CHAIN_ID` / `PAYMENT_AMOUNT` don't see the UI mislead users before the wallet opens.

## Payment Signing Shape

The frontend signs the same EIP-712 domain and type enforced by the verifier:

```text
Domain:
  name: MicroAI Paygate
  version: 1
  chainId: paymentContext.chainId
  verifyingContract: 0x0000000000000000000000000000000000000000

Payment:
  recipient address
  token string
  amount string
  nonce string
  timestamp uint256
```

If this shape changes, update gateway, verifier, web, E2E tests, OpenAPI, and docs together.

## Local Development

Install dependencies:

```bash
cd web
bun install
```

Run the app:

```bash
bun run dev
```

Open `http://localhost:3001`.

The gateway must be reachable at `NEXT_PUBLIC_GATEWAY_URL` or `http://localhost:3000`.

## Production Build

```bash
cd web
bun run lint
bun run build
bun run test
```

`bun run test` runs `tsc --noEmit`.

## Deployment Notes

`web/vercel.json` configures Vercel to install with Bun and build with `bun run build`. Set `NEXT_PUBLIC_GATEWAY_URL` in Vercel project environment settings; do not hard-code the real gateway URL in committed files.

When linking the Vercel project, use `web` as the project root.
