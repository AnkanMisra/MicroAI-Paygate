import { ethers } from "ethers";
import type { PaymentContext, PaymentContextV2, PaymentSigner } from "./protocol/types";

export const PAYMENT_DOMAIN_NAME = "MicroAI Paygate";
export const PAYMENT_DOMAIN_VERSION = "1";
export const PAYMENT_DOMAIN_VERSION_V2 = "2";

export const PAYMENT_TYPES = {
  Payment: [
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
  ],
};

export const PAYMENT_AUTHORIZATION_TYPES = {
  PaymentAuthorization: [
    { name: "payer", type: "address" },
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
    { name: "audience", type: "string" },
    { name: "method", type: "string" },
    { name: "resource", type: "string" },
    { name: "contentType", type: "string" },
    { name: "requestHash", type: "bytes32" },
  ],
};

export function isPaymentContextV2(ctx: PaymentContext): ctx is PaymentContextV2 {
  return "authorizationVersion" in ctx && ctx.authorizationVersion === 2;
}

export function buildPaymentTypedData(ctx: PaymentContext, payer?: string) {
  if (isPaymentContextV2(ctx)) {
    if (!payer) {
      throw new Error("Request-bound payment authorization requires a payer address");
    }
    return {
      domain: {
        name: PAYMENT_DOMAIN_NAME,
        version: PAYMENT_DOMAIN_VERSION_V2,
        chainId: ctx.chainId,
        verifyingContract: ethers.ZeroAddress,
      },
      types: PAYMENT_AUTHORIZATION_TYPES,
      value: {
        payer,
        recipient: ctx.recipient,
        token: ctx.token,
        amount: ctx.amount,
        nonce: ctx.nonce,
        timestamp: ctx.timestamp,
        audience: ctx.audience,
        method: ctx.method,
        resource: ctx.resource,
        contentType: ctx.contentType,
        requestHash: ctx.requestHash,
      },
    };
  }

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
  payer?: string,
): Promise<string> {
  const resolvedPayer = isPaymentContextV2(ctx) ? payer ?? (await signer.getAddress?.()) : undefined;
  const { domain, types, value } = buildPaymentTypedData(ctx, resolvedPayer);
  return signer.signTypedData(domain, types, value);
}

export function buildSignedHeaders(
  ctx: PaymentContext,
  signature: string,
  payer?: string,
): Record<string, string> {
  const headers: Record<string, string> = {
    "X-402-Signature": signature,
    "X-402-Nonce": ctx.nonce,
    "X-402-Timestamp": ctx.timestamp.toString(),
  };
  if (isPaymentContextV2(ctx)) {
    if (!payer) {
      throw new Error("Request-bound payment authorization requires a payer address");
    }
    headers["X-402-Payer"] = payer;
  }
  return headers;
}
