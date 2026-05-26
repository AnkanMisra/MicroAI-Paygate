# @microai/paygate-sdk

Local TypeScript SDK for the current MicroAI Paygate x402-style protocol.

The SDK handles the existing gateway flow:

1. Send an unsigned request.
2. Read the `402 Payment Required` `paymentContext`.
3. Sign the payment context with EIP-712.
4. Retry with `X-402-Signature`, `X-402-Nonce`, and `X-402-Timestamp`.
5. Decode the `X-402-Receipt` response header.
6. Verify the signed receipt locally.

This package is private and local for now. It is not published to npm.

## Protocol Status

MicroAI Paygate currently uses a custom x402-style wire contract. It is not official x402-compatible yet because it uses custom `X-402-*` headers and does not perform facilitator-backed or on-chain USDC settlement.

A valid EIP-712 signature proves wallet authorization for the payment context. It does not prove that USDC moved on-chain.

## Install And Test

```bash
cd sdk/typescript
bun install
bun run test
```

Run the type checker:

```bash
bun run typecheck
```

## Usage

```ts
import { ethers } from "ethers";
import { PaygateClient } from "@microai/paygate-sdk";

const client = new PaygateClient({
  gatewayUrl: "http://localhost:3000",
  signer: new ethers.Wallet(process.env.EVM_PRIVATE_KEY!),
  trustedServerPublicKey: process.env.PAYGATE_SERVER_PUBLIC_KEY,
});

const response = await client.summarize("Text to summarize");

console.log(response.data.result);
console.log(response.receipt?.receipt.id);
console.log(response.receiptVerified);
```

Generic endpoint usage:

```ts
const response = await client.request<{ text: string }, { result: string }>({
  method: "POST",
  path: "/api/ai/summarize",
  body: { text: "..." },
});
```

## Example

Set environment variables:

```text
PAYGATE_GATEWAY_URL=http://localhost:3000
EVM_PRIVATE_KEY=0x...
PAYGATE_SERVER_PUBLIC_KEY=0x...
```

Use only unfunded local or test wallets. Never use a funded wallet, seed phrase, production key, or real customer wallet in examples.

`PAYGATE_SERVER_PUBLIC_KEY` should be the gateway receipt signing public key distributed out of band. Without it, the SDK can decode receipts and verify request/response hash binding, but it returns `receiptVerified: false` instead of trusting the self-declared key inside the receipt.

Run:

```bash
cd sdk/typescript
bun run examples/summarize.ts "Text to summarize"
```

## Optional Live Test

The live SDK test is skipped by default. Start the local stack first:

```bash
bun run stack
```

Then run:

```bash
PAYGATE_SDK_LIVE_TEST=1 EVM_PRIVATE_KEY=0x... PAYGATE_SERVER_PUBLIC_KEY=0x... PAYGATE_GATEWAY_URL=http://localhost:3000 bun test src/__tests__/live-gateway.test.ts
```

The live path depends on the gateway's configured AI provider. With the default OpenRouter provider, the gateway still needs `OPENROUTER_API_KEY`.
