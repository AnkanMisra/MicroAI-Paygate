import { ethers } from "ethers";
import { v4 as uuidv4 } from "uuid";
import { CONFIG } from "../config";

// In-memory store for nonces (production should use Redis/DB)
const nonceStore = new Set<string>();

export interface PaymentContext {
  recipient: string;
  token: string;
  amount: string;
  chainId: number;
  nonce: string;
  timestamp: number;
}

// EIP-712 Domain Separator
const DOMAIN = {
  name: "MicroAI Paygate",
  version: "1",
  chainId: CONFIG.CHAIN_ID,
  verifyingContract: ethers.ZeroAddress, // No specific contract for this off-chain sig, or use a facilitator contract address
};

// EIP-712 Types
const TYPES = {
  Payment: [
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
  ],
};

export class PaymentService {
  /**
   * Generates a new payment context with a unique nonce.
   */
  static createPaymentContext(price: string = CONFIG.PAYMENT.DEFAULT_PRICE): PaymentContext {
    const nonce = uuidv4();
    nonceStore.add(nonce);

    return {
      recipient: CONFIG.PAYMENT.RECIPIENT_ADDRESS,
      token: CONFIG.PAYMENT.TOKEN_SYMBOL,
      amount: price,
      chainId: CONFIG.CHAIN_ID,
      nonce,
      timestamp: Math.floor(Date.now() / 1000),
    };
  }

  /**
   * Verifies the EIP-712 signature for a payment.
   */
  static async verifyPayment(
    context: PaymentContext,
    signature: string
  ): Promise<{ isValid: boolean; error?: string; signer?: string }> {
    try {
      // 1. Check Nonce
      if (!nonceStore.has(context.nonce)) {
        return { isValid: false, error: "Invalid or expired nonce" };
      }

      // 2. Verify Signature
      // Reconstruct the data that was signed
      const value = {
        recipient: context.recipient,
        token: context.token,
        amount: context.amount,
        nonce: context.nonce,
        timestamp: context.timestamp,
      };

      const signerAddress = ethers.verifyTypedData(DOMAIN, TYPES, value, signature);

      // 3. Consume Nonce (prevent replay)
      nonceStore.delete(context.nonce);

      return { isValid: true, signer: signerAddress };
    } catch (error) {
      console.error("Payment verification failed:", error);
      return { isValid: false, error: "Signature verification failed" };
    }
  }
}
