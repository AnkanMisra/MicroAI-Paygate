# Web Frontend

The Web Frontend is the user-facing interface for MicroAI Paygate. Built with Next.js, it manages the user journey from inputting text to signing crypto transactions.

## Role & Responsibilities

- **User Interface**: Provides a clean, responsive UI for text summarization.
- **Wallet Integration**: Connects to EVM-compatible browser wallets (MetaMask, Rabby, Coinbase Wallet) using `ethers.js`.
- **Payment Flow Handling**:
    1.  Sends initial request.
    2.  Catches `402 Payment Required` errors.
    3.  Prompts user to sign EIP-712 typed data.
    4.  Retries request with signature headers.
- **Network Management**: Automatically detects network mismatches and prompts switching to the Base Sepolia network.

## Technology Stack

- **Framework**: Next.js 16 (App Router)
- **Language**: TypeScript
- **Styling**: Tailwind CSS
- **Blockchain Interaction**: Ethers.js v6

## Key Files

- `src/app/page.tsx`: The main application logic, including state management and the `handleSummarize` function.
- `Dockerfile`: Configuration for building the Next.js application for production.

## Development

To run the frontend locally:

```bash
bun run dev
```

The application will be available at `http://localhost:3001`.

## Configuration

Environment variables (place in `.env.local` or `.env`):

- `NEXT_PUBLIC_GATEWAY_URL` — gateway base URL (e.g., http://localhost:3000)
- `NEXT_PUBLIC_CHAIN_ID` — chain id for EIP-712 domain (align with gateway/verifier)
- `NEXT_PUBLIC_RPC_URL` — RPC endpoint for wallet network detection/switching
- `NEXT_PUBLIC_RECIPIENT` — expected recipient address for payments

## Payment Flow

1) Send summarize request to gateway. 2) Receive `402 Payment Required`. 3) Sign EIP-712 typed data in-browser. 4) Retry with `X-402-Signature` and `X-402-Nonce`. 5) Display AI result or failure if the upstream model call fails.

## Testing

Frontend E2E coverage is driven from the root `tests/e2e.test.ts`; ensure the gateway and verifier are reachable at the configured URLs when running it.
