import { ethers, type JsonRpcSigner } from "ethers";
import type { PaymentContext } from "./types";
import { validateReceiptFormat, type SignedReceipt } from "./verify-receipt";

const DOMAIN_NAME = "MicroAI Paygate";
const DOMAIN_VERSION = "1";

export function getGatewayUrl(): string {
  return process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:3000";
}

export async function postSummarize(
  text: string,
  headers: Record<string, string> = {},
): Promise<Response> {
  return fetch(`${getGatewayUrl()}/api/ai/summarize`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...headers },
    body: JSON.stringify({ text }),
  });
}

export async function readPaymentChallenge(res: Response): Promise<PaymentContext> {
  const data = (await res.json()) as { paymentContext?: PaymentContext };
  if (!data.paymentContext) throw new Error("402 response missing paymentContext");
  return data.paymentContext;
}

export async function signPaymentContext(
  signer: JsonRpcSigner,
  ctx: PaymentContext,
): Promise<string> {
  const domain = {
    name: DOMAIN_NAME,
    version: DOMAIN_VERSION,
    chainId: ctx.chainId,
    verifyingContract: ethers.ZeroAddress,
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
  const value = {
    recipient: ctx.recipient,
    token: ctx.token,
    amount: ctx.amount,
    nonce: ctx.nonce,
    timestamp: ctx.timestamp,
  };
  return signer.signTypedData(domain, types, value);
}

export function buildSignedHeaders(ctx: PaymentContext, signature: string): Record<string, string> {
  return {
    "X-402-Signature": signature,
    "X-402-Nonce": ctx.nonce,
    "X-402-Timestamp": ctx.timestamp.toString(),
  };
}

export type SummarizeSuccess = {
  summary: string;
  receipt: SignedReceipt | null;
};

export async function readSummarizeSuccess(res: Response): Promise<SummarizeSuccess> {
  const data = (await res.json()) as { result?: string };
  const summary = data.result ?? "";

  const headerVal = res.headers.get("x-402-receipt") ?? res.headers.get("X-402-Receipt");
  const receipt = headerVal ? safeDecodeReceiptHeader(headerVal) : null;
  return { summary, receipt };
}

export function safeDecodeReceiptHeader(b64: string): SignedReceipt | null {
  let decoded: unknown;
  try {
    decoded = JSON.parse(atob(b64));
  } catch (err) {
    console.warn("Failed to decode X-402-Receipt header", err);
    return null;
  }

  // Shape-validate BEFORE handing back to useX402's success path. Without
  // this, a malformed header (gateway bug, mid-flight tamper, schema drift)
  // would pass through as a "SignedReceipt", saveReceipt would dereference
  // .receipt.id on a partial object, and the outer catch would replace a
  // successful paid summary with an "unknown" error.
  //
  // validateReceiptFormat uses optional-chained .startsWith() — if a field
  // is the wrong TYPE (number, object) instead of missing entirely, the
  // method call throws. Wrap defensively so any unexpected shape just
  // drops the receipt instead of bubbling out and losing the paid summary.
  let ok = false;
  try {
    ok = validateReceiptFormat(decoded as SignedReceipt);
  } catch (err) {
    console.warn("validateReceiptFormat threw on decoded X-402-Receipt", err);
    return null;
  }
  if (!ok) {
    console.warn("X-402-Receipt header decoded to malformed SignedReceipt; dropping");
    return null;
  }
  return decoded as SignedReceipt;
}
