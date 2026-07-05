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
// Kept for backwards compatibility; the gateway now embeds the receipt in the
// SSE stream body as `data: {"receipt": "<base64>"}` rather than this header.
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

/**
 * Extracts the base64-encoded receipt from an SSE stream body string.
 * The gateway emits: `data: {"receipt": "<base64>"}\n\n`
 * Returns null if no receipt event is found.
 */
export function extractReceiptFromSseBody(bodyText: string): SignedReceipt | null {
  const lines = bodyText.split("\n");
  for (const line of lines) {
    if (!line.startsWith("data: ")) continue;
    const dataStr = line.slice("data: ".length).trim();
    if (dataStr === "[DONE]" || dataStr === "") continue;
    try {
      const parsed: unknown = JSON.parse(dataStr);
      if (isRecord(parsed) && typeof parsed.receipt === "string") {
        return decodeReceiptHeader(parsed.receipt);
      }
    } catch {
      // skip malformed lines
    }
  }
  return null;
}

/**
 * Consumes an SSE stream response, accumulating text chunks and extracting
 * the embedded receipt event. Returns the full summary text, the receipt,
 * and the canonical response body JSON string used for receipt hash binding.
 */
export async function readSseSuccessBody(response: Response): Promise<{
  fullText: string;
  receipt: SignedReceipt | null;
  canonicalResponseBody: string;
}> {
  const bodyText = await response.text();
  let fullText = "";
  let receipt: SignedReceipt | null = null;

  const lines = bodyText.split("\n");
  for (const line of lines) {
    if (!line.startsWith("data: ")) continue;
    const dataStr = line.slice("data: ".length).trim();
    if (dataStr === "[DONE]" || dataStr === "") continue;
    try {
      const parsed: unknown = JSON.parse(dataStr);
      if (!isRecord(parsed)) continue;
      if (typeof parsed.text === "string") {
        fullText += parsed.text;
      } else if (typeof parsed.receipt === "string") {
        receipt = decodeReceiptHeader(parsed.receipt);
      } else if (parsed.error !== undefined) {
        throw new PaygateSdkError(
          "network_error",
          `Gateway stream error: ${String(parsed.error)}`,
          {},
        );
      }
    } catch (e) {
      if (e instanceof PaygateSdkError) throw e;
      // ignore unparseable lines
    }
  }

  // Build the canonical response body that the gateway hashes for the receipt.
  // GenerateReceipt in the gateway uses: json.Marshal({"result": fullSummary})
  const canonicalResponseBody = JSON.stringify({ result: fullText });

  return { fullText, receipt, canonicalResponseBody };
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

  // readReceipt is no longer used for SSE responses; receipt extraction is
  // handled by readSseSuccessBody in client.ts. This method is retained for
  // protocol-adapter interface compatibility.
  readReceipt(_response: Response): SignedReceipt | null {
    return null;
  }
}
