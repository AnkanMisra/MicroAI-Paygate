import { ethers } from "ethers";
import { decodeBase64ToUtf8 } from "./base64";
import { PaygateSdkError } from "./errors";
import type { FetchLike, Receipt, SignedReceipt } from "./protocol/types";

export type VerifyReceiptOptions = {
  expectedServerPublicKey: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function isPrefixedString(value: unknown, prefix: string): value is string {
  return isNonEmptyString(value) && value.startsWith(prefix);
}

export function validateReceiptFormat(value: unknown): value is SignedReceipt {
  if (!isRecord(value)) return false;
  const receipt = value.receipt;
  if (!isRecord(receipt)) return false;

  const payment = receipt.payment;
  const service = receipt.service;
  if (!isRecord(payment) || !isRecord(service)) return false;

  return (
    isPrefixedString(receipt.id, "rcpt_") &&
    isNonEmptyString(receipt.version) &&
    isNonEmptyString(receipt.timestamp) &&
    isNonEmptyString(payment.payer) &&
    isNonEmptyString(payment.recipient) &&
    isNonEmptyString(payment.amount) &&
    isNonEmptyString(payment.token) &&
    typeof payment.chainId === "number" &&
    Number.isSafeInteger(payment.chainId) &&
    payment.chainId > 0 &&
    isNonEmptyString(payment.nonce) &&
    isNonEmptyString(service.endpoint) &&
    isPrefixedString(service.request_hash, "sha256:") &&
    isPrefixedString(service.response_hash, "sha256:") &&
    isPrefixedString(value.signature, "0x") &&
    isPrefixedString(value.server_public_key, "0x")
  );
}

export function decodeReceiptHeader(headerValue: string): SignedReceipt {
  try {
    const json = decodeBase64ToUtf8(headerValue);
    const decoded = JSON.parse(json) as unknown;
    if (!validateReceiptFormat(decoded)) {
      throw new Error("decoded receipt does not match SignedReceipt shape");
    }
    return decoded;
  } catch (error) {
    throw new PaygateSdkError(
      "receipt_decode_failed",
      "Failed to decode X-402-Receipt as a gateway SignedReceipt",
      { cause: error },
    );
  }
}

function serializeReceiptForGateway(receipt: Receipt): string {
  return JSON.stringify({
    id: receipt.id,
    version: receipt.version,
    timestamp: receipt.timestamp,
    payment: {
      payer: receipt.payment.payer,
      recipient: receipt.payment.recipient,
      amount: receipt.payment.amount,
      token: receipt.payment.token,
      chainId: receipt.payment.chainId,
      nonce: receipt.payment.nonce,
    },
    service: {
      endpoint: receipt.service.endpoint,
      request_hash: receipt.service.request_hash,
      response_hash: receipt.service.response_hash,
    },
  });
}

function normalizePublicKey(value: string | undefined): string | null {
  if (!value) return null;
  try {
    return ethers.SigningKey.computePublicKey(value, false).toLowerCase();
  } catch {
    return null;
  }
}

export async function verifyReceipt(
  signedReceipt: SignedReceipt,
  options?: VerifyReceiptOptions,
): Promise<boolean> {
  try {
    if (!validateReceiptFormat(signedReceipt)) return false;
    const expectedPublicKey = normalizePublicKey(options?.expectedServerPublicKey);
    if (expectedPublicKey === null) return false;

    const receiptPublicKey = normalizePublicKey(signedReceipt.server_public_key);
    if (receiptPublicKey !== expectedPublicKey) return false;

    const receiptJson = serializeReceiptForGateway(signedReceipt.receipt);
    const messageHash = ethers.keccak256(ethers.toUtf8Bytes(receiptJson));
    const sigBytes = ethers.getBytes(signedReceipt.signature);
    if (sigBytes.length !== 65) return false;

    const recoveryId = sigBytes[64];
    const v = recoveryId <= 1 ? recoveryId + 27 : recoveryId;
    const signature = ethers.Signature.from({
      r: ethers.hexlify(sigBytes.slice(0, 32)),
      s: ethers.hexlify(sigBytes.slice(32, 64)),
      v,
    });

    const recoveredPubKey = ethers.SigningKey.recoverPublicKey(messageHash, signature);
    return normalizePublicKey(recoveredPubKey) === expectedPublicKey;
  } catch {
    return false;
  }
}

export async function fetchReceipt(
  receiptId: string,
  gatewayUrl = "http://localhost:3000",
  fetcher: FetchLike = globalThis.fetch.bind(globalThis),
): Promise<SignedReceipt | null> {
  try {
    const response = await fetcher(
      `${gatewayUrl.replace(/\/+$/, "")}/api/receipts/${encodeURIComponent(receiptId)}`,
    );
    if (response.status === 404) return null;

    if (!response.ok) {
      throw new PaygateSdkError("network_error", "Failed to fetch receipt", {
        status: response.status,
        bodyText: await response.text(),
      });
    }

    let body: unknown;
    try {
      body = await response.json();
    } catch (error) {
      throw new PaygateSdkError("receipt_decode_failed", "Receipt lookup response was not JSON", {
        status: response.status,
        cause: error,
      });
    }

    if (!validateReceiptFormat(body)) {
      throw new PaygateSdkError(
        "receipt_decode_failed",
        "Receipt lookup response did not match SignedReceipt shape",
        { status: response.status },
      );
    }
    return body;
  } catch (error) {
    if (error instanceof PaygateSdkError) throw error;
    throw new PaygateSdkError("network_error", "Network error while fetching receipt", {
      cause: error,
    });
  }
}
