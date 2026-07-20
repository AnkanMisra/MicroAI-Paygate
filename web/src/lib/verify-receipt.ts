/**
 * Receipt Verification Library for MicroAI-Paygate
 * 
 * Verifies cryptographic receipts using ECDSA signatures and Keccak256 hashing.
 * Compatible with Ethereum wallet signatures.
 * 
 * @module verify-receipt
 */

import { ethers } from 'ethers';
import type { PaymentContext } from './types';

// Type definitions matching backend Go structs

export interface PaymentDetails {
  payer: string;
  recipient: string;
  amount: string;
  token: string;
  chainId: number;
  nonce: string;
  timestamp?: number;
}

export interface ServiceDetails {
  endpoint: string;
  authorization_version?: number;
  audience?: string;
  method?: string;
  resource?: string;
  content_type?: string;
  authorization_request_hash?: string;
  request_hash: string;
  response_hash: string;
}

export interface Receipt {
  id: string;
  version: string;
  timestamp: string;
  payment: PaymentDetails;
  service: ServiceDetails;
}

export interface SignedReceipt {
  receipt: Receipt;
  signature: string;
  server_public_key: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function hasExactKeys(value: Record<string, unknown>, keys: string[]): boolean {
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

function serializeReceiptForGateway(receipt: Receipt): string {
  const service = receipt.version === '1.0'
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
      ...(receipt.version === '2.0' && { timestamp: receipt.payment.timestamp }),
    },
    service,
  });
}

function stringifyLikeGo(value: unknown): string {
  return JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, (character) =>
    `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`,
  );
}

export function receiptMatchesPaymentAuthorization(
  signedReceipt: SignedReceipt,
  authorization: PaymentContext,
  payer: string,
): boolean {
  if (authorization.authorizationVersion !== 2) return false;
  const payment = signedReceipt.receipt.payment;
  const service = signedReceipt.receipt.service;
  try {
    return signedReceipt.receipt.version === '2.0' &&
      ethers.getAddress(payment.payer) === ethers.getAddress(payer) &&
      ethers.getAddress(payment.recipient) === ethers.getAddress(authorization.recipient) &&
      payment.amount === authorization.amount &&
      payment.token === authorization.token &&
      payment.chainId === authorization.chainId &&
      payment.nonce === authorization.nonce &&
      payment.timestamp === authorization.timestamp &&
      service.authorization_version === authorization.authorizationVersion &&
      service.audience === authorization.audience &&
      service.method === authorization.method &&
      service.resource === authorization.resource &&
      service.content_type === authorization.contentType &&
      service.authorization_request_hash === authorization.requestHash;
  } catch {
    return false;
  }
}

function sha256Body(bodyText: string): string {
  return `sha256:${ethers.sha256(ethers.toUtf8Bytes(bodyText)).slice(2)}`;
}

export function receiptMatchesExchange(
  signedReceipt: SignedReceipt,
  endpoint: string,
  requestBodyText: string,
  responseBodyText: string,
): boolean {
  const service = signedReceipt.receipt.service;
  return service.endpoint === endpoint &&
    service.request_hash === sha256Body(requestBodyText) &&
    service.response_hash === sha256Body(responseBodyText);
}

/**
 * Verifies a cryptographic receipt signature
 * 
 * @param signedReceipt - The signed receipt from the API response
 * @returns Promise<boolean> - true if signature is valid
 * 
 * @example
 * ```typescript
 * const response = await fetch('/api/ai/summarize', { ...headers... });
 * const data = await response.json();
 * const isValid = await verifyReceipt(data.receipt);
 * console.log(`Receipt valid: ${isValid}`);
 * ```
 */
export async function verifyReceipt(signedReceipt: SignedReceipt): Promise<boolean> {
  try {
    // Validate structure
    if (!validateReceiptFormat(signedReceipt)) {
      console.error('Invalid receipt structure');
      return false;
    }

    // Serialize according to the versioned Go receipt contract.
    const receiptJSON = serializeReceiptForGateway(signedReceipt.receipt);
    
    // Hash using Keccak256 (Ethereum-compatible) - same as Go's crypto.Keccak256Hash
    const messageHash = ethers.keccak256(ethers.toUtf8Bytes(receiptJSON));

    // Convert signature from hex string to bytes
    const sigBytes = ethers.getBytes(signedReceipt.signature);

    // Go's crypto.Sign produces 65-byte signatures: [R (32 bytes)][S (32 bytes)][V (1 byte)]
   // V is the recovery ID (0 or 1 in Go, 27 or 28 in Ethereum)
    if (sigBytes.length !== 65) {
      console.error(`Invalid signature length: expected 65 bytes, got ${sigBytes.length}`);
      return false;
    }

    // Recover the public key from the signature
    // Go uses v=0/1, but ethers expects v=27/28, so we add 27
    const signature = ethers.Signature.from({
      r: ethers.hexlify(sigBytes.slice(0, 32)),
      s: ethers.hexlify(sigBytes.slice(32, 64)),
      v: sigBytes[64] + 27
    });

    const recoveredPubKey = ethers.SigningKey.recoverPublicKey(messageHash, signature);

    // Compare recovered public key with server's public key
    // Both should be uncompressed public keys (0x04 prefix + 64 bytes)
    return recoveredPubKey.toLowerCase() === signedReceipt.server_public_key.toLowerCase();
  } catch (error) {
    console.error('Receipt verification failed:', error);
    return false;
  }
}

/**
 * Validates receipt format without verifying signature
 * 
 * @param signedReceipt - The receipt to validate
 * @returns boolean - true if format is valid
 */
export function validateReceiptFormat(value: unknown): value is SignedReceipt {
  if (!isRecord(value) || !isRecord(value.receipt)) return false;
  const signedReceipt = value as unknown as SignedReceipt;

  const r = signedReceipt.receipt;
  if (!isRecord(r.payment) || !isRecord(r.service)) return false;
  
  const validBase = !!(
    hasExactKeys(r as unknown as Record<string, unknown>, ['id', 'version', 'timestamp', 'payment', 'service']) &&
    r.id?.startsWith('rcpt_') &&
    (r.version === '1.0' || r.version === '2.0') &&
    r.timestamp &&
    r.payment?.payer &&
    r.payment?.recipient &&
    r.payment?.amount &&
    r.payment?.token &&
    r.payment?.nonce &&
    r.service?.endpoint &&
    r.service?.request_hash &&
    r.service?.response_hash &&
    signedReceipt.signature?.startsWith('0x') &&
    signedReceipt.server_public_key?.startsWith('0x')
  );
  if (!validBase) return false;

  const v2Fields = [
    r.service.authorization_version,
    r.service.audience,
    r.service.method,
    r.service.resource,
    r.service.content_type,
    r.service.authorization_request_hash,
  ];
  if (r.version === '1.0') {
    return hasExactKeys(r.payment as unknown as Record<string, unknown>, ['payer', 'recipient', 'amount', 'token', 'chainId', 'nonce']) &&
      hasExactKeys(r.service as unknown as Record<string, unknown>, ['endpoint', 'request_hash', 'response_hash']) &&
      v2Fields.every((field) => field === undefined);
  }
  return hasExactKeys(r.payment as unknown as Record<string, unknown>, ['payer', 'recipient', 'amount', 'token', 'chainId', 'nonce', 'timestamp']) &&
    hasExactKeys(r.service as unknown as Record<string, unknown>, ['endpoint', 'authorization_version', 'audience', 'method', 'resource', 'content_type', 'authorization_request_hash', 'request_hash', 'response_hash']) &&
    typeof r.payment.timestamp === 'number' && Number.isSafeInteger(r.payment.timestamp) && r.payment.timestamp > 0 &&
    r.service.authorization_version === 2 &&
    [
      r.service.audience,
      r.service.method,
      r.service.resource,
      r.service.content_type,
      r.service.authorization_request_hash,
    ].every((field) => typeof field === 'string' && field.length > 0) &&
    r.service.authorization_request_hash!.startsWith('0x');
}

/**
 * Fetches a receipt by ID from the gateway
 * 
 * @param receiptId - Receipt ID (e.g., "rcpt_a1b2c3d4e5f6")
 * @param gatewayUrl - Gateway base URL (default: http://localhost:3000)
 * @returns Promise<SignedReceipt | null>
 */
export async function fetchReceipt(
  receiptId: string,
  gatewayUrl: string = 'http://localhost:3000'
): Promise<SignedReceipt | null> {
  try {
    const response = await fetch(`${gatewayUrl}/api/receipts/${receiptId}`);
    
    if (response.status === 404) {
      return null;
    }
    
    if (!response.ok) {
      throw new Error(`Failed to fetch receipt: ${response.statusText}`);
    }
    
    const data = await response.json();
    
    return {
      receipt: data.receipt,
      signature: data.signature,
      server_public_key: data.server_public_key,
    };
  } catch (error) {
    console.error('Error fetching receipt:', error);
    return null;
  }
}
