import {
  ethers,
  type JsonRpcSigner,
  type TypedDataDomain,
  type TypedDataField,
} from "ethers";
import type { PaymentContext, PaymentContextV2 } from "./types";
import { validateReceiptFormat, type SignedReceipt } from "./verify-receipt";

const DOMAIN_NAME = "MicroAI Paygate";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function isPaymentContext(value: unknown): value is PaymentContext {
  if (!isRecord(value)) return false;
  if (
    !isNonEmptyString(value.recipient) ||
    !isNonEmptyString(value.token) ||
    !isNonEmptyString(value.amount) ||
    !isNonEmptyString(value.nonce) ||
    !isPositiveSafeInteger(value.chainId) ||
    !isPositiveSafeInteger(value.timestamp)
  ) {
    return false;
  }
  if (value.authorizationVersion === undefined) {
    return !["audience", "method", "resource", "contentType", "requestHash"].some(
      (field) => field in value,
    );
  }
  return (
    value.authorizationVersion === 2 &&
    isNonEmptyString(value.audience) &&
    isNonEmptyString(value.method) &&
    isNonEmptyString(value.resource) &&
    isNonEmptyString(value.contentType) &&
    typeof value.requestHash === "string" &&
    /^0x[0-9a-fA-F]{64}$/.test(value.requestHash)
  );
}

function isPaymentContextV2(ctx: PaymentContext): ctx is PaymentContextV2 {
  return "authorizationVersion" in ctx && ctx.authorizationVersion === 2;
}

export function getGatewayUrl(): string {
  return process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:3000";
}

export function getSummarizeUrl(): string {
  return `${getGatewayUrl().replace(/\/+$/, "")}/api/ai/summarize`;
}

export function serializeSummarizeRequest(text: string): string {
  return JSON.stringify({ text });
}

export async function postSummarize(
  bodyText: string,
  headers: Record<string, string> = {},
): Promise<Response> {
  return fetch(getSummarizeUrl(), {
    method: "POST",
    headers: { "Content-Type": "application/json", ...headers },
    body: bodyText,
  });
}

export async function readPaymentChallenge(res: Response): Promise<PaymentContext> {
  const data: unknown = await res.json();
  const paymentContext = isRecord(data) ? data.paymentContext : undefined;
  if (!isPaymentContext(paymentContext)) {
    throw new Error("402 response missing a valid paymentContext");
  }
  return paymentContext;
}

export type PaymentRequestBinding = {
  url: string;
  method: string;
  contentType: string;
  bodyText: string;
};

export async function getPaymentPayer(
  signer: JsonRpcSigner,
  ctx: PaymentContext,
): Promise<string | undefined> {
  return isPaymentContextV2(ctx) ? signer.getAddress() : undefined;
}

export function validatePaymentContextForRequest(
  ctx: PaymentContext,
  request: PaymentRequestBinding,
): void {
  if (!isPaymentContextV2(ctx)) return;
  const url = new URL(request.url);
  const expected = {
    audience: url.origin,
    method: request.method.toUpperCase(),
    resource: `${url.pathname}${url.search}`,
    contentType: request.contentType,
    requestHash: ethers.sha256(ethers.toUtf8Bytes(request.bodyText)),
  };
  for (const field of [
    "audience",
    "method",
    "resource",
    "contentType",
    "requestHash",
  ] as const) {
    if (ctx[field] !== expected[field]) {
      throw new Error(`Payment challenge ${field} does not match the outgoing request`);
    }
  }
}

type PaymentTypedData = {
  domain: TypedDataDomain;
  types: Record<string, TypedDataField[]>;
  value: Record<string, unknown>;
};

export function buildPaymentTypedData(ctx: PaymentContext, payer?: string): PaymentTypedData {
  if (isPaymentContextV2(ctx)) {
    if (!payer) {
      throw new Error("Request-bound payment authorization requires a payer address");
    }
    return {
      domain: {
        name: DOMAIN_NAME,
        version: "2",
        chainId: ctx.chainId,
        verifyingContract: ethers.ZeroAddress,
      },
      types: {
        PaymentAuthorization: [
          { name: "payer", type: "address" },
          { name: "recipient", type: "address" },
          { name: "token", type: "string" },
          { name: "amount", type: "string" },
          { name: "nonce", type: "string" },
          { name: "timestamp", type: "uint256" },
          { name: "audience", type: "string" },
          { name: "method", type: "string" },
          { name: "resource", type: "string" },
          { name: "contentType", type: "string" },
          { name: "requestHash", type: "bytes32" },
        ],
      },
      value: {
        payer,
        recipient: ctx.recipient,
        token: ctx.token,
        amount: ctx.amount,
        nonce: ctx.nonce,
        timestamp: ctx.timestamp,
        audience: ctx.audience,
        method: ctx.method,
        resource: ctx.resource,
        contentType: ctx.contentType,
        requestHash: ctx.requestHash,
      },
    };
  }

  return {
    domain: {
      name: DOMAIN_NAME,
      version: "1",
      chainId: ctx.chainId,
      verifyingContract: ethers.ZeroAddress,
    },
    types: {
      Payment: [
        { name: "recipient", type: "address" },
        { name: "token", type: "string" },
        { name: "amount", type: "string" },
        { name: "nonce", type: "string" },
        { name: "timestamp", type: "uint256" },
      ],
    },
    value: {
      recipient: ctx.recipient,
      token: ctx.token,
      amount: ctx.amount,
      nonce: ctx.nonce,
      timestamp: ctx.timestamp,
    },
  };
}

export async function signPaymentContext(
  signer: JsonRpcSigner,
  ctx: PaymentContext,
  payer?: string,
): Promise<string> {
  const resolvedPayer = payer ?? (await getPaymentPayer(signer, ctx));
  const { domain, types, value } = buildPaymentTypedData(ctx, resolvedPayer);
  return signer.signTypedData(domain, types, value);
}

export function buildSignedHeaders(
  ctx: PaymentContext,
  signature: string,
  payer?: string,
): Record<string, string> {
  const headers: Record<string, string> = {
    "X-402-Signature": signature,
    "X-402-Nonce": ctx.nonce,
    "X-402-Timestamp": ctx.timestamp.toString(),
  };
  if (isPaymentContextV2(ctx)) {
    if (!payer) {
      throw new Error("Request-bound payment authorization requires a payer address");
    }
    headers["X-402-Payer"] = payer;
  }
  return headers;
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
