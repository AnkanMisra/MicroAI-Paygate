import { ethers } from "ethers";
import { buildSignedHeaders, signPaymentContext } from "../payment";
import { decodeReceiptHeader } from "../receipts";
import { PaygateSdkError } from "../errors";
import type {
  PaygateProtocolAdapter,
  PaymentContext,
  PaymentContextV2,
  PaymentRequestBinding,
  PaymentSigner,
  SignedReceipt,
} from "./types";

export const MICROAI_SIGNATURE_HEADER = "X-402-Signature";
export const MICROAI_NONCE_HEADER = "X-402-Nonce";
export const MICROAI_TIMESTAMP_HEADER = "X-402-Timestamp";
export const MICROAI_PAYER_HEADER = "X-402-Payer";
export const MICROAI_RECEIPT_HEADER = "X-402-Receipt";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function hasBasePaymentFields(value: Record<string, unknown>): boolean {
  return (
    isNonEmptyString(value.recipient) &&
    isNonEmptyString(value.token) &&
    isNonEmptyString(value.amount) &&
    isNonEmptyString(value.nonce) &&
    isPositiveSafeInteger(value.chainId) &&
    isPositiveSafeInteger(value.timestamp)
  );
}

function isPaymentContext(value: unknown): value is PaymentContext {
  if (!isRecord(value)) return false;
  if (!hasBasePaymentFields(value)) return false;
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

export function validatePaymentContextForRequest(
  ctx: PaymentContext,
  request: PaymentRequestBinding,
): void {
  if (ctx.authorizationVersion !== 2) return;

  const url = new URL(request.url);
  const expected: Pick<
    PaymentContextV2,
    "audience" | "method" | "resource" | "contentType" | "requestHash"
  > = {
    audience: url.origin,
    method: request.method.toUpperCase(),
    resource: `${url.pathname}${url.search}`,
    contentType: request.contentType,
    requestHash: ethers.sha256(
      request.bodyText === undefined
        ? new Uint8Array()
        : ethers.toUtf8Bytes(request.bodyText),
    ),
  };

  for (const field of [
    "audience",
    "method",
    "resource",
    "contentType",
    "requestHash",
  ] as const) {
    if (ctx[field] !== expected[field]) {
      throw new PaygateSdkError(
        "payment_binding_mismatch",
        `Payment challenge ${field} does not match the outgoing request`,
      );
    }
  }
}

export class MicroAIPaygateProtocol implements PaygateProtocolAdapter {
  async readPaymentContext(response: Response): Promise<PaymentContext> {
    const bodyText = await response.text();
    let parsed: unknown;
    try {
      parsed = JSON.parse(bodyText);
    } catch (error) {
      throw new PaygateSdkError(
        "payment_challenge_missing",
        "402 response did not contain JSON paymentContext",
        { status: response.status, bodyText, cause: error },
      );
    }

    const paymentContext = isRecord(parsed) ? parsed.paymentContext : undefined;
    if (!isPaymentContext(paymentContext)) {
      throw new PaygateSdkError(
        "payment_challenge_missing",
        "402 response is missing a valid paymentContext",
        { status: response.status, bodyText },
      );
    }

    return paymentContext;
  }

  validatePaymentContext(ctx: PaymentContext, request: PaymentRequestBinding): void {
    validatePaymentContextForRequest(ctx, request);
  }

  async getPayer(signer: PaymentSigner, ctx: PaymentContext): Promise<string | undefined> {
    if (ctx.authorizationVersion !== 2) return undefined;
    if (!signer.getAddress) {
      throw new PaygateSdkError(
        "payment_signature_failed",
        "The signer must expose getAddress() for request-bound authorization",
      );
    }
    return signer.getAddress();
  }

  signPaymentContext(
    signer: PaymentSigner,
    ctx: PaymentContext,
    payer?: string,
  ): Promise<string> {
    return signPaymentContext(signer, ctx, payer);
  }

  buildSignedHeaders(
    ctx: PaymentContext,
    signature: string,
    payer?: string,
  ): Record<string, string> {
    return buildSignedHeaders(ctx, signature, payer);
  }

  readReceipt(response: Response): SignedReceipt | null {
    const header = response.headers.get(MICROAI_RECEIPT_HEADER);
    return header ? decodeReceiptHeader(header) : null;
  }
}
