# Deployment

Free-tier deployment of MicroAI Paygate across three platforms. Total recurring cost: **$0**. No credit card required.

## Topology

| Service | Host | Plan | Notes |
| --- | --- | --- | --- |
| `verifier/` (Rust) | Render Web Service | Free | Public over HTTPS; stateless EIP-712 recovery |
| `gateway/` (Go) | Render Web Service | Free | Public; calls verifier and OpenRouter, talks to Redis |
| `web/` (Next.js) | Vercel | Hobby | Built from `web/` subdirectory |
| Redis | Upstash | Free | Receipt store + nonce replay protection |

Both Render services share a region for low inter-service latency. Both **sleep after 15 minutes of inactivity** — the first request after sleep takes 30–50 seconds while the containers wake. The web app shows a warm-up banner during this window. See [Cold-start behavior](#cold-start-behavior) for details.

## Prerequisites

You'll need accounts on:

- [Render](https://render.com) — GitHub OAuth sign-up, **no card required**
- [Vercel](https://vercel.com) — GitHub OAuth sign-up, no card required
- [Upstash](https://upstash.com) — GitHub OAuth sign-up, no card required
- [OpenRouter](https://openrouter.ai) — free models are available (`z-ai/glm-4.5-air:free` by default)

And these values in hand:

- `OPENROUTER_API_KEY` from OpenRouter
- `SERVER_WALLET_PRIVATE_KEY` — any 64-hex private key (demo only, never holds real funds)
- `RECIPIENT_ADDRESS` — the EIP-55-checksummed address derived from the key above
- `REDIS_URL` from Upstash (format: `rediss://default:...@...upstash.io:6379`)

Optional but recommended CLIs:

```sh
brew install render vercel-cli
render login    # GitHub OAuth
vercel login    # GitHub OAuth
render workspace set
```

## 1. Provision Upstash Redis

1. Sign up at https://upstash.com using GitHub OAuth.
2. Console → **Create Database** → choose **Regional**, pick the region nearest your eventual Render region (e.g. Mumbai/`bom` or Singapore/`sin`), keep TLS enabled, leave eviction **off** for nonce-replay correctness.
3. Open the database → **Connect to your database** → copy the `rediss://` URL (TLS). Save it — this is `REDIS_URL`.

Quick sanity check:

```sh
redis-cli -u 'rediss://default:...@...upstash.io:6379' PING   # should print PONG
```

## 2. Deploy the Verifier on Render

The verifier is stateless EIP-712 signature recovery. Public on Render's free tier — acceptable because it exposes no secrets and only does cryptographic recovery on caller-supplied inputs.

1. Render dashboard → **New +** → **Web Service** → connect `AnkanMisra/MicroAI-Paygate`.
2. Configure:

| Field | Value |
| --- | --- |
| Name | `microai-verifier` (or similar; becomes the URL subdomain) |
| Language | Docker |
| Branch | `main` |
| Root Directory | `verifier` |
| Region | Singapore (`aws-ap-southeast-1`) — closest free region for Mumbai Upstash; pick whatever matches your Upstash region |
| Instance Type | **Free** (default is paid — must change) |
| Docker Build Context Directory | `verifier` |
| Dockerfile Path | `verifier/Dockerfile` |
| Health Check Path | `/health` |

3. Environment variables (Advanced section):
   - `CHAIN_ID=84532`
   - `PORT=3002`
   - `REDIS_URL=<your-upstash-rediss-url>`
   - `VERIFIER_NONCE_STORE=redis`
   - `VERIFIER_NONCE_KEY_PREFIX=microai:verifier:nonce:`
   - `VERIFIER_REDIS_TIMEOUT_MS=2000`

4. Click **Deploy Web Service**. First Rust build takes ~3–5 min.
5. Copy the assigned public URL — e.g. `https://microai-verifier.onrender.com`. The gateway needs this URL in the next step.

Verify:

```sh
curl https://<verifier-app>.onrender.com/health
# {"status":"healthy","service":"verifier","version":"..."}
```

## 3. Deploy the Gateway on Render

1. Render dashboard → **New +** → **Web Service** → same repo.
2. Configure:

| Field | Value |
| --- | --- |
| Name | `microai-gateway` (must differ from the verifier's name) |
| Language | Docker |
| Branch | `main` |
| Root Directory | `gateway` |
| Region | **same as verifier** |
| Instance Type | **Free** |
| Docker Build Context Directory | `gateway` |
| Dockerfile Path | `gateway/Dockerfile` |
| Health Check Path | `/healthz` (note the trailing `z` — differs from the verifier) |

3. Environment variables. Use Render's **"Add from .env"** button and paste the block below, then fill in the four `<...>` placeholders with real values:

```env
OPENROUTER_API_KEY=<your-openrouter-key>
OPENROUTER_MODEL=z-ai/glm-4.5-air:free
SERVER_WALLET_PRIVATE_KEY=<your-64-hex-key>
RECIPIENT_ADDRESS=<your-eip55-checksummed-address>
REDIS_URL=<your-upstash-rediss-url>
RECEIPT_STORE=redis
VERIFIER_URL=https://<verifier-app>.onrender.com
CHAIN_ID=84532
EXPECTED_CHAIN_ID=84532
PAYMENT_AMOUNT=0.001
ALLOWED_ORIGINS=*
TRUSTED_PROXIES=0.0.0.0/0
VERIFIER_TIMEOUT_SECONDS=60
PORT=3000
```

> **Important:** `RECIPIENT_ADDRESS` must be the canonical EIP-55-checksummed form, not lowercased or arbitrary-case. The browser wallet rejects malformed checksums with `bad address checksum` during signing.
>
> **CORS:** `ALLOWED_ORIGINS=*` is permissive; tighten it after web deploy in step 5.
>
> **Verifier timeout:** the default `2s` is too short for Render free-tier cold-starts. `60s` lets the verifier wake up during the first signed request.

4. Click **Deploy Web Service**. Go builds take ~2 min.
5. Verify:

```sh
curl https://<gateway-app>.onrender.com/healthz
# {"service":"gateway","status":"ok","timestamp":...}
```

6. Test the 402 challenge:

```sh
curl -i https://<gateway-app>.onrender.com/api/ai/summarize \
  -H 'Content-Type: application/json' \
  -d '{"text":"hello"}'
# HTTP 402
# {"error":"Payment Required","paymentContext":{...}}
```

## 4. Deploy the Web on Vercel

1. Sign up at https://vercel.com using GitHub OAuth.
2. **Add New** → **Project** → import `AnkanMisra/MicroAI-Paygate`.
3. Configure:

| Field | Value |
| --- | --- |
| Project Name | `microai-paygate` (becomes the Vercel subdomain) |
| Framework | Next.js (auto-detected) |
| Root Directory | `web` |

4. Environment Variables — add:
   - `NEXT_PUBLIC_GATEWAY_URL` = `https://<gateway-app>.onrender.com`
   - `NEXT_PUBLIC_VERIFIER_URL` = `https://<verifier-app>.onrender.com` (lets the warm-up banner pre-wake the verifier too)

The four other `NEXT_PUBLIC_*` vars (`EXPECTED_CHAIN_ID`, `EXPECTED_CHAIN_NAME`, `PAYMENT_AMOUNT`, `PAYMENT_TOKEN`) have correct defaults baked into the code for Base Sepolia + USDC + 0.001. Set them only if you deploy against a non-default chain.

5. **Deploy**. ~1–2 min.
6. Copy the assigned URL — e.g. `https://microai-paygate.vercel.app`.

## 5. Tighten Gateway CORS

Go back to the gateway service on Render → **Environment** → update `ALLOWED_ORIGINS` from `*` to your exact Vercel domain:

```
ALLOWED_ORIGINS=https://<your-vercel-app>.vercel.app
```

This triggers an auto-redeploy. After ~1 min, preflight requests from the Vercel origin should still succeed; requests from any other origin will be blocked.

Verify:

```sh
curl -sI -X OPTIONS \
  -H 'Origin: https://<your-vercel-app>.vercel.app' \
  -H 'Access-Control-Request-Method: POST' \
  https://<gateway-app>.onrender.com/api/ai/summarize | grep -i access-control-allow-origin
# access-control-allow-origin: https://<your-vercel-app>.vercel.app
```

## 6. Smoke Test the Full Flow

1. Open `https://<your-vercel-app>.vercel.app` in a browser.
2. You may see a "§ Free tier wake-up" banner at the top while the warm-up pings resolve.
3. Connect MetaMask (or any EIP-1193 wallet) on Base Sepolia.
4. Paste a paragraph of text into the form.
5. Click **Sign & Summarize**.
6. Wallet pops up — sign the EIP-712 typed-data payment context.
7. Wait for: verifier validates → gateway calls OpenRouter → receipt is signed and returned.
8. The receipt panel shows: summary text, signed receipt JSON, and a client-side signature verification badge.

If any step errors, check Render service logs for the gateway and verifier — both are visible from the Render dashboard.

## Cold-start behavior

Render's free tier sleeps web services after 15 minutes of no traffic. The first request after sleep takes 30–50 seconds while the container restarts. Mitigations baked into this project:

- `web/src/components/cold-start-warmup.tsx` pings both `gateway/healthz` and `verifier/health` in parallel on first page load and shows a banner until they resolve. Subsequent requests are normal speed.
- `VERIFIER_TIMEOUT_SECONDS=60` on the gateway tolerates verifier cold-starts during signed requests.

These are good enough for portfolio / demo use. For an always-on production deployment, upgrade Render's Starter plan ($7/mo per service) or pivot to Fly.io (requires a card).

## Updating env vars after initial deploy

Use either the Render dashboard or the CLI:

```sh
# Render CLI — list services
render services

# Render REST API — update one env var (no CLI command yet exists for this)
RENDER_TOKEN=$(grep '^    key:' ~/.render/cli.yaml | awk '{print $2}')
curl -s -X PUT \
  -H "Authorization: Bearer $RENDER_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg v 'new-value' '{value: $v}')" \
  "https://api.render.com/v1/services/<service-id>/env-vars/<KEY>"

# Trigger a redeploy
render deploys create <service-id>
```

For Vercel env vars, use the CLI from the `web/` directory:

```sh
vercel env add NEXT_PUBLIC_FOO production
vercel env rm  NEXT_PUBLIC_FOO production
```

## Secret Handling

- `.env.production.example` and committed `*.yaml` files contain placeholders only.
- Real values belong only in Render service env vars and Vercel project env vars.
- Never commit OpenRouter keys, private keys, Upstash URLs, or wallet material.
- Audit `.env*` files before any push: they should appear in `.gitignore`.

## Alternative platforms

If you'd rather pay for always-on hosting:

- **Fly.io** — requires a card on file but the same architecture maps cleanly (private networking between gateway and verifier via `.internal` DNS).
- **Railway**, **Koyeb**, **Hugging Face Spaces** — all support Docker. Refer to each platform's docs.

This guide is opinionated toward Render + Vercel + Upstash because that combination is genuinely zero-cost-zero-card and produces resume-credible URLs.
