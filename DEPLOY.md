# Deployment Prep

Phase 3 prepares deployment files and operator instructions only. Do not run the Fly app creation, Fly secret, Fly deploy, Vercel env, or Vercel deploy commands until Phase 4 manual review approves real production values.

## Topology

- Gateway: public Fly app on port `3000`, health check `GET /healthz`
- Verifier: private Fly app on port `3002`, reachable only over Fly internal DNS at `http://<verifier-app>.internal:3002`
- Web: Vercel-hosted Next.js app
- Redis: Upstash Redis for receipts and optional response cache

The verifier stays private because the gateway is the only public caller. Keeping verifier traffic on Fly internal DNS reduces the exposed cryptographic verification surface.

Keep the verifier at one Machine until replay nonce storage is moved to a shared store such as Redis. The current verifier uses in-process nonce replay protection.

## Prerequisites

- Fly CLI authenticated with access to the target org
- Vercel CLI authenticated with access to the target project
- Upstash Redis database URL
- OpenRouter API key
- Base Sepolia demo server wallet private key
- Base Sepolia recipient address
- Reviewed copies of `deploy/fly/gateway.fly.toml`, `deploy/fly/verifier.fly.toml`, and `.env.production.example`

## 1. Choose App Names

Replace these placeholders before running any command:

```sh
export FLY_ORG=<your-fly-org>
export GATEWAY_APP=<gateway-app>
export VERIFIER_APP=<verifier-app>
export VERCEL_APP_URL=https://<your-vercel-app>.vercel.app
```

Update `app = "<gateway-app>"` in `deploy/fly/gateway.fly.toml`.
Update `app = "<verifier-app>"` in `deploy/fly/verifier.fly.toml`.
Update `VERIFIER_URL = "http://<verifier-app>.internal:3002"` in `deploy/fly/gateway.fly.toml`.

Base Sepolia is the default chain for this deployment prep. For any other chain, update `CHAIN_ID` and `EXPECTED_CHAIN_ID` together in `.env.production.example`, `deploy/fly/gateway.fly.toml`, and `deploy/fly/verifier.fly.toml` so the gateway and verifier cannot drift.

## 2. Create Fly Apps

Phase 3 does not run these commands. They are prepared for Phase 4.

```sh
fly apps create <verifier-app> --org <your-fly-org>
fly apps create <gateway-app> --org <your-fly-org>
```

## 3. Provision Upstash Redis

Create an Upstash Redis database in the Upstash console. Use TLS when available and copy the production Redis URL, for example:

```text
<your-upstash-redis-url>
```

Set it only through Fly secrets. Do not commit the real URL to this repository.

## 4. Set Fly Secrets

Use placeholders until Phase 4. Do not commit real secret values.

```sh
fly secrets set -a <gateway-app> \
  OPENROUTER_API_KEY=<your-openrouter-api-key> \
  SERVER_WALLET_PRIVATE_KEY=<your-demo-server-wallet-private-key> \
  RECIPIENT_ADDRESS=<your-base-sepolia-recipient-address> \
  REDIS_URL=<your-upstash-redis-url>
```

Non-secret gateway settings such as `OPENROUTER_MODEL`, `PAYMENT_AMOUNT`, and `ALLOWED_ORIGINS` live in `deploy/fly/gateway.fly.toml`. Edit those placeholders before deploying instead of setting them as Fly secrets.

If you later enable `RATE_LIMIT_ENABLED=true` on Fly, configure trusted Fly proxy CIDRs first so IP-based rate limiting uses real client IPs instead of a shared proxy address:

```sh
fly secrets set -a <gateway-app> \
  TRUSTED_PROXIES=<fly-proxy-cidr>
```

Mirror non-secret runtime values from `.env.production.example` in the Fly config files and use Fly secrets only for secret values or values you intentionally do not want committed. Keep `CHAIN_ID` and `EXPECTED_CHAIN_ID` synchronized.

The verifier currently needs no secret material. Its non-secret defaults are committed in `deploy/fly/verifier.fly.toml`:

```sh
fly secrets list -a <verifier-app>
```

## 5. Deploy Verifier

Deploy verifier before gateway so the gateway can reach `http://<verifier-app>.internal:3002`.

```sh
(cd verifier && fly deploy -c ../deploy/fly/verifier.fly.toml -a <verifier-app>)
fly scale count 1 -a <verifier-app>
```

## 6. Deploy Gateway

```sh
(cd gateway && fly deploy -c ../deploy/fly/gateway.fly.toml -a <gateway-app>)
```

Fly should route public HTTPS traffic to the gateway app and use `GET /healthz` for stable liveness during cold starts. Use `/readyz` manually when checking dependency readiness because it also checks verifier, provider, and Redis reachability.

## 7. Configure Vercel

Do not hard-code the real gateway URL in `web/vercel.json`. Set it in Vercel project settings or through the CLI:

```sh
cd web
vercel link
vercel env add NEXT_PUBLIC_GATEWAY_URL production
```

When prompted for `NEXT_PUBLIC_GATEWAY_URL`, enter:

```text
https://<gateway-app>.fly.dev
```

If you are deploying with a non-default chain (e.g. Base mainnet rather than Base Sepolia) or a non-default fee, you must also set the matching public display vars so the wallet widget, hero headline, and stat bar agree with the gateway's payment context. All four are `NEXT_PUBLIC_*` so they're inlined into the Next.js bundle at build time:

```sh
vercel env add NEXT_PUBLIC_EXPECTED_CHAIN_ID production    # e.g. 8453 for Base mainnet
vercel env add NEXT_PUBLIC_EXPECTED_CHAIN_NAME production  # e.g. Base
vercel env add NEXT_PUBLIC_PAYMENT_AMOUNT production       # e.g. 0.001
vercel env add NEXT_PUBLIC_PAYMENT_TOKEN production        # e.g. USDC
```

`NEXT_PUBLIC_EXPECTED_CHAIN_ID` must match the gateway's `CHAIN_ID`, and `NEXT_PUBLIC_EXPECTED_CHAIN_NAME` must match it (e.g. `8453` paired with `Base`, not `Base Sepolia`) — otherwise the UI will contradict itself. Defaults are Base Sepolia (`84532`, `Base Sepolia`) and `0.001 USDC`.

Run the CLI commands from `web/`. If you link from the repository root or configure the project in the Vercel dashboard, set the project root directory to `web`.

Deploy the web app after the env is configured:

```sh
vercel deploy --prod
```

## 8. Smoke Tests

Run these only after Phase 4 deploys real apps and secrets.

```sh
curl -fsS https://<gateway-app>.fly.dev/healthz
```

An unsigned summarize request should return `402` with a payment context:

```sh
curl -i https://<gateway-app>.fly.dev/api/ai/summarize \
  -H 'Content-Type: application/json' \
  -d '{"text":"Summarize this deployment smoke test."}'
```

Then use the Vercel web app with a Base Sepolia wallet:

1. Open `https://<your-vercel-app>.vercel.app`.
2. Submit a summarize request.
3. Confirm the wallet is on Base Sepolia.
4. Sign the EIP-712 payment request.
5. Confirm the signed retry returns `200`.
6. Capture the `X-402-Receipt` value from the response.

Decode the receipt header before using it in the lookup endpoint. `X-402-Receipt` is base64-encoded signed receipt JSON; the lookup endpoint expects the embedded receipt ID.

```sh
RECEIPT_B64=<x-402-receipt-header-value>
RECEIPT_ID=$(printf '%s' "$RECEIPT_B64" | python3 -c 'import base64,json,sys; print(json.loads(base64.b64decode(sys.stdin.read().strip()))["receipt"]["id"])')
```

Verify receipt persistence after a gateway restart or deploy:

```sh
curl -i https://<gateway-app>.fly.dev/api/receipts/$RECEIPT_ID
```

The receipt should remain retrievable after the gateway Machine is restarted or a new gateway deploy completes, because production uses `RECEIPT_STORE=redis` backed by Upstash Redis.

## Secret Handling

- Committed files contain placeholders only.
- Real values belong in Fly secrets or Vercel environment settings.
- Never commit OpenRouter keys, private keys, Upstash Redis URLs, or production wallet material.
- Review `.env.production.example` before copying values into any secret store.
