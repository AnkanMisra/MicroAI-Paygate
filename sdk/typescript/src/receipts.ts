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

function hasExactKeys(value: Record<string, unknown>, keys: string[]): boolean {
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

export function validateReceiptFormat(value: unknown): value is SignedReceipt {
  if (!isRecord(value)) return false;
  const receipt = value.receipt;
  if (!isRecord(receipt)) return false;

  const payment = receipt.payment;
  const service = receipt.service;
  if (!isRecord(payment) || !isRecord(service)) return false;

  const validBase =
    hasExactKeys(receipt, ["id", "version", "timestamp", "payment", "service"]) &&
    isPrefixedString(receipt.id, "rcpt_") &&
    (receipt.version === "1.0" || receipt.version === "2.0") &&
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
    isPrefixedString(value.server_public_key, "0x");
  if (!validBase) return false;

  const v2Fields = [
    service.audience,
    service.method,
    service.resource,
    service.content_type,
    service.authorization_request_hash,
  ];
  if (receipt.version === "1.0") {
    return (
      hasExactKeys(payment, ["payer", "recipient", "amount", "token", "chainId", "nonce"]) &&
      hasExactKeys(service, ["endpoint", "request_hash", "response_hash"]) &&
      service.authorization_version === undefined &&
      v2Fields.every((field) => field === undefined)
    );
  }
  return (
    hasExactKeys(payment, ["payer", "recipient", "amount", "token", "chainId", "nonce", "timestamp"]) &&
    hasExactKeys(service, ["endpoint", "authorization_version", "audience", "method", "resource", "content_type", "authorization_request_hash", "request_hash", "response_hash"]) &&
    typeof payment.timestamp === "number" &&
    Number.isSafeInteger(payment.timestamp) &&
    payment.timestamp > 0 &&
    service.authorization_version === 2 &&
    v2Fields.every(isNonEmptyString) &&
    isPrefixedString(service.authorization_request_hash, "0x")
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
  const service =
    receipt.version === "1.0"
      ? {
          endpoint: receipt.service.endpoint,
          request_hash: receipt.service.request_hash,
          response_hash: receipt.service.response_hash,
        }
      : {
          endpoint: receipt.service.endpoint,
          authorization_version: receipt.service.authorization_version,
          audience: receipt.service.audience,
          method: receipt.service.method,
          resource: receipt.service.resource,
          content_type: receipt.service.content_type,
          authorization_request_hash: receipt.service.authorization_request_hash,
          request_hash: receipt.service.request_hash,
          response_hash: receipt.service.response_hash,
        };
  return stringifyLikeGo({
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
      ...(receipt.version === "2.0" && { timestamp: receipt.payment.timestamp }),
    },
    service,
  });
}

function stringifyLikeGo(value: unknown): string {
  return JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, (character) =>
    `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
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
