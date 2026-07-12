import { buildSignedHeaders, signPaymentContext } from "../payment";
import { decodeReceiptHeader } from "../receipts";
import { PaygateSdkError } from "../errors";
import type {
  PaygateProtocolAdapter,
  PaymentContext,
  PaymentSigner,
  SignedReceipt,
} from "./types";

export const MICROAI_SIGNATURE_HEADER = "X-402-Signature";
export const MICROAI_NONCE_HEADER = "X-402-Nonce";
export const MICROAI_TIMESTAMP_HEADER = "X-402-Timestamp";
export const MICROAI_RECEIPT_HEADER = "X-402-Receipt";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function isPaymentContext(value: unknown): value is PaymentContext {
  if (!isRecord(value)) return false;
  return (
    typeof value.recipient === "string" &&
    typeof value.token === "string" &&
    typeof value.amount === "string" &&
    typeof value.nonce === "string" &&
    isPositiveSafeInteger(value.chainId) &&
    isPositiveSafeInteger(value.timestamp)
  );
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

  signPaymentContext(signer: PaymentSigner, ctx: PaymentContext): Promise<string> {
    return signPaymentContext(signer, ctx);
  }

  buildSignedHeaders(ctx: PaymentContext, signature: string): Record<string, string> {
    return buildSignedHeaders(ctx, signature);
  }

  readReceipt(response: Response): SignedReceipt | null {
    const header = response.headers.get(MICROAI_RECEIPT_HEADER);
    return header ? decodeReceiptHeader(header) : null;
  }
}
