import { ethers } from "ethers";
import type { PaymentContext, PaymentSigner } from "./protocol/types";

export const PAYMENT_DOMAIN_NAME = "MicroAI Paygate";
export const PAYMENT_DOMAIN_VERSION = "1";

export const PAYMENT_TYPES = {
  Payment: [
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
  ],
};

export function buildPaymentTypedData(ctx: PaymentContext) {
  return {
    domain: {
      name: PAYMENT_DOMAIN_NAME,
      version: PAYMENT_DOMAIN_VERSION,
      chainId: ctx.chainId,
      verifyingContract: ethers.ZeroAddress,
    },
    types: PAYMENT_TYPES,
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
  signer: PaymentSigner,
  ctx: PaymentContext,
): Promise<string> {
  const { domain, types, value } = buildPaymentTypedData(ctx);
  return signer.signTypedData(domain, types, value);
}

export function buildSignedHeaders(ctx: PaymentContext, signature: string): Record<string, string> {
  return {
    "X-402-Signature": signature,
    "X-402-Nonce": ctx.nonce,
    "X-402-Timestamp": ctx.timestamp.toString(),
  };
}
