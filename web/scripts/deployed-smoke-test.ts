/**
 * End-to-end smoke test against a deployed MicroAI Paygate stack.
 *
 * Generates an ephemeral wallet, walks the full x402 flow (challenge ->
 * EIP-712 sign -> retry with X-402 headers -> receipt), and probes a
 * handful of negative cases (replay, expired timestamp, wrong-origin
 * CORS, malformed verifier input).
 *
 * Run:
 *   cd web && bun run scripts/deployed-smoke-test.ts
 *
 * Override targets with env vars (defaults match the deployed demo):
 *   GATEWAY_URL=https://your-gateway.onrender.com
 *   VERIFIER_URL=https://your-verifier.onrender.com
 *   ORIGIN=https://your-web.vercel.app
 *
 * Exits 0 on full success, 1 if any happy-path assertion fails.
 */

import { ethers } from "ethers";

const GATEWAY = process.env.GATEWAY_URL ?? "https://microai-gateway.onrender.com";
const VERIFIER = process.env.VERIFIER_URL ?? "https://microai-paygate.onrender.com";
const ORIGIN = process.env.ORIGIN ?? "https://microai-paygate.vercel.app";

const DOMAIN_NAME = "MicroAI Paygate";
const DOMAIN_VERSION = "1";

type PaymentContext = {
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
};

const types = {
  Payment: [
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
  ],
};

let failures = 0;
function rec(label: string, ok: boolean, detail: string) {
  console.log(`  ${ok ? "PASS" : "FAIL"}  ${label.padEnd(45)} ${detail}`);
  if (!ok) failures++;
}
function bar(s: string) {
  console.log(`\n${"=".repeat(70)}\n${s}\n${"=".repeat(70)}`);
}

// Wraps every network call with an AbortSignal timeout so a sleeping
// Render free-tier service produces a clear failure instead of hanging
// silently past the script's wall-clock budget.
const FETCH_TIMEOUT_MS = 60_000;
function timedFetch(url: string, init: RequestInit = {}): Promise<Response> {
  return fetch(url, { ...init, signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) });
}

async function getChallenge(text = "smoke"): Promise<PaymentContext> {
  const r = await timedFetch(`${GATEWAY}/api/ai/summarize`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Origin: ORIGIN },
    body: JSON.stringify({ text }),
  });
  return (await r.json()).paymentContext as PaymentContext;
}

async function signCtx(wallet: ethers.Signer, ctx: PaymentContext, chainIdOverride?: number) {
  return wallet.signTypedData(
    {
      name: DOMAIN_NAME,
      version: DOMAIN_VERSION,
      chainId: chainIdOverride ?? ctx.chainId,
      verifyingContract: ethers.ZeroAddress,
    },
    types,
    {
      recipient: ctx.recipient,
      token: ctx.token,
      amount: ctx.amount,
      nonce: ctx.nonce,
      timestamp: ctx.timestamp,
    },
  );
}

async function postSigned(ctx: PaymentContext, sig: string) {
  const r = await timedFetch(`${GATEWAY}/api/ai/summarize`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Origin: ORIGIN,
      "X-402-Signature": sig,
      "X-402-Nonce": ctx.nonce,
      "X-402-Timestamp": String(ctx.timestamp),
    },
    body: JSON.stringify({ text: "smoke test summarize" }),
  });
  return { status: r.status, body: await r.text(), headers: r.headers };
}

async function main() {
  console.log(`Targets:\n  gateway  ${GATEWAY}\n  verifier ${VERIFIER}\n  origin   ${ORIGIN}`);
  const wallet = ethers.Wallet.createRandom();
  console.log(`Ephemeral wallet: ${wallet.address}`);

  bar("Happy path");
  const ctx = await getChallenge();
  rec("recipient is EIP-55 canonical", (() => { try { return ethers.getAddress(ctx.recipient) === ctx.recipient; } catch { return false; } })(), ctx.recipient);
  const sig = await signCtx(wallet, ctx);
  const ok = await postSigned(ctx, sig);
  rec("signed flow returns 200", ok.status === 200, `HTTP ${ok.status}`);
  const receiptHeader = ok.headers.get("x-402-receipt");
  rec("X-402-Receipt header present", !!receiptHeader, receiptHeader ? "yes" : "missing");

  let receiptId: string | undefined;
  if (receiptHeader) {
    const decoded = JSON.parse(atob(receiptHeader));
    const r = decoded.receipt;
    receiptId = r?.id;
    rec("receipt has id + signature", !!r?.id && !!decoded.signature, r?.id ?? "?");
    rec("receipt.payment.payer matches signer", r?.payment?.payer?.toLowerCase() === wallet.address.toLowerCase(), r?.payment?.payer ?? "(missing)");
  }

  bar("Negative cases");
  const replay = await postSigned(ctx, sig);
  rec("replay rejected with 409", replay.status === 409, `HTTP ${replay.status}`);

  const expiredCtx = await getChallenge();
  expiredCtx.timestamp = Math.floor(Date.now() / 1000) - 3600;
  const expiredSig = await signCtx(wallet, expiredCtx);
  const expired = await postSigned(expiredCtx, expiredSig);
  rec("expired timestamp rejected with 400", expired.status === 400, `HTTP ${expired.status}`);

  // Asserts on the CORS header rather than the HTTP status: Bun's
  // server-side fetch doesn't enforce CORS, so a 200 here would still
  // be browser-blocked as long as Access-Control-Allow-Origin is absent.
  // Checking the header is more portable across CORS middleware impls
  // (some 403, some return 200 with no allow header).
  const wrongOrigin = await timedFetch(`${GATEWAY}/api/ai/summarize`, {
    method: "OPTIONS",
    headers: { Origin: "https://evil.example.com", "Access-Control-Request-Method": "POST" },
  });
  const wrongAllowOrigin = wrongOrigin.headers.get("access-control-allow-origin");
  rec(
    "CORS preflight from wrong origin omits allow-origin",
    wrongAllowOrigin === null,
    wrongAllowOrigin ?? `(no allow-origin) HTTP ${wrongOrigin.status}`,
  );

  const garbageVerify = await timedFetch(`${VERIFIER}/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: '{"foo":"bar"}',
  });
  rec("verifier rejects garbage with 400", garbageVerify.status === 400, `HTTP ${garbageVerify.status}`);

  bar("Receipt persistence");
  if (receiptId) {
    const lookup = await timedFetch(`${GATEWAY}/api/receipts/${receiptId}`);
    rec("receipt lookup returns 200", lookup.status === 200, `HTTP ${lookup.status}`);
  } else {
    rec("receipt lookup", false, "skipped — no receipt id");
  }

  bar(`Summary: ${failures} failure(s)`);
  process.exit(failures === 0 ? 0 : 1);
}

main().catch((e) => {
  console.error("FATAL:", e);
  process.exit(2);
});
