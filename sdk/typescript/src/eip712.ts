/**
 * EIP-712 signing utilities for MicroAI-Paygate
 */

import { ethers } from 'ethers';
import type { PaymentContext } from './types.js';

/**
 * EIP-712 domain for MicroAI-Paygate
 */
export function getEIP712Domain(chainId: number) {
  return {
    name: 'MicroAI-Paygate',
    version: '1',
    chainId,
  };
}

/**
 * EIP-712 types for PaymentContext
 */
export const PaymentContextTypes = {
  PaymentContext: [
    { name: 'recipient', type: 'address' },
    { name: 'token', type: 'string' },
    { name: 'amount', type: 'string' },
    { name: 'nonce', type: 'string' },
    { name: 'chainId', type: 'uint256' },
  ],
};

/**
 * Sign a payment context using EIP-712
 * 
 * @param signer - Ethers signer
 * @param context - Payment context from 402 response
 * @returns Hex-encoded signature
 * 
 * @example
 * ```typescript
 * const signature = await signPaymentContext(signer, {
 *   recipient: '0x...',
 *   token: 'USDC',
 *   amount: '0.001',
 *   nonce: 'abc123',
 *   chainId: 8453
 * });
 * ```
 */
export async function signPaymentContext(
  signer: ethers.Signer,
  context: PaymentContext
): Promise<string> {
  // Get domain with chainId from context
  const domain = getEIP712Domain(context.chainId);

  // Sign using EIP-712
  const signature = await signer.signTypedData(
    domain,
    PaymentContextTypes,
    context
  );

  return signature;
}

/**
 * Verify an EIP-712 signature (for testing)
 * 
 * @param context - Payment context
 * @param signature - Signature to verify
 * @returns Recovered signer address
 */
export function recoverSigner(
  context: PaymentContext,
  signature: string
): string {
  const domain = getEIP712Domain(context.chainId);

  const recoveredAddress = ethers.verifyTypedData(
    domain,
    PaymentContextTypes,
    context,
    signature
  );

  return recoveredAddress;
}
