/**
 * Receipt Verification Library for MicroAI-Paygate
 * 
 * Verifies cryptographic receipts using ECDSA signatures and Keccak256 hashing.
 * Compatible with Ethereum wallet signatures.
 * 
 * @module verify-receipt
 */

import { ethers } from 'ethers';

// Type definitions matching backend Go structs

export interface PaymentDetails {
  payer: string;
  recipient: string;
  amount: string;
  token: string;
  chainId: number;
  nonce: string;
}

export interface ServiceDetails {
  endpoint: string;
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
    if (!signedReceipt?.receipt || !signedReceipt.signature || !signedReceipt.server_public_key) {
      console.error('Invalid receipt structure');
      return false;
    }

    // Serialize receipt deterministically (alphabetical by JSON tag, same as Go)
    const receiptJSON = JSON.stringify(signedReceipt.receipt);
    
    // Hash using Keccak256 (Ethereum-compatible)
    const messageHash = ethers.keccak256(ethers.toUtf8Bytes(receiptJSON));

    // Recover signer address from signature
    const recoveredAddress = ethers.recoverAddress(messageHash, signedReceipt.signature);

    // Compute address from server's public key
    const serverAddress = ethers.computeAddress(signedReceipt.server_public_key);

    // Compare addresses (case-insensitive)
    return recoveredAddress.toLowerCase() === serverAddress.toLowerCase();
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
export function validateReceiptFormat(signedReceipt: SignedReceipt): boolean {
  if (!signedReceipt?.receipt) return false;
  
  const r = signedReceipt.receipt;
  
  return !!(
    r.id?.startsWith('rcpt_') &&
    r.version &&
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
}

/**
 * Fetches a receipt by ID from the gateway
 * 
 * @param receiptId - Receipt ID (e.g., "rcpt_abc123")
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
