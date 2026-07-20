import { describe, expect, it } from 'bun:test';
import { ethers } from 'ethers';
import {
  receiptMatchesPaymentAuthorization,
  validateReceiptFormat,
  verifyReceipt,
  type Receipt,
  type SignedReceipt,
} from './verify-receipt';
import type { PaymentContextV2 } from './types';

const signingKey = new ethers.SigningKey(
  '0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
);

function signedReceipt(receipt: Receipt): SignedReceipt {
  const goJSON = JSON.stringify(receipt).replace(/[<>&\u2028\u2029]/g, (character) =>
    `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`,
  );
  const digest = ethers.keccak256(ethers.toUtf8Bytes(goJSON));
  const signature = signingKey.sign(digest);
  return {
    receipt,
    signature: ethers.hexlify(
      ethers.concat([signature.r, signature.s, ethers.toBeHex(signature.yParity, 1)]),
    ),
    server_public_key: signingKey.publicKey,
  };
}

function receipt(version: '1.0' | '2.0'): Receipt {
  const service = {
    endpoint: '/api/ai/summarize',
    ...(version === '2.0' && {
      authorization_version: 2,
      audience: 'https://gateway.example.com',
      method: 'POST',
      resource: '/api/ai/summarize?mode=brief&tag=<x>\u2028',
      content_type: 'application/json',
      authorization_request_hash:
        '0x8187d0879ad19b46b277e1b761d3f70d51bc9de6459530b686cfaa503ae8d0e9',
    }),
    request_hash: 'sha256:8187d0879ad19b46b277e1b761d3f70d51bc9de6459530b686cfaa503ae8d0e9',
    response_hash: 'sha256:8a90fd4352d6e287b3e908e62f802c99c4f5680c9644cb27fb64d638e3fbb9d4',
  };
  return {
    id: 'rcpt_abcdef123456',
    version,
    timestamp: '2026-07-20T00:00:00Z',
    payment: {
      payer: '0x14791697260E4c9A71f18484C9f997B308e59325',
      recipient: '0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219',
      amount: '0.001',
      token: 'USDC',
      chainId: 84532,
      nonce: 'receipt-version-test',
      ...(version === '2.0' && { timestamp: 1760918400 }),
    },
    service,
  };
}

describe('versioned receipt verification', () => {
  it('verifies stored v1 and request-bound v2 receipt serialization', async () => {
    expect(await verifyReceipt(signedReceipt(receipt('1.0')))).toBe(true);
    expect(await verifyReceipt(signedReceipt(receipt('2.0')))).toBe(true);
  });

  it('rejects unsupported versions and v2 metadata under version 1.0', () => {
    const unsupported = signedReceipt(receipt('1.0'));
    unsupported.receipt.version = '3.0';
    expect(validateReceiptFormat(unsupported)).toBe(false);

    const legacyWithV2Metadata = signedReceipt(receipt('1.0'));
    legacyWithV2Metadata.receipt.service.authorization_version = 2;
    expect(validateReceiptFormat(legacyWithV2Metadata)).toBe(false);

    const extraProperty = signedReceipt(receipt('1.0')) as SignedReceipt & {
      receipt: Receipt & { status?: string };
    };
    extraProperty.receipt.status = 'refunded';
    expect(validateReceiptFormat(extraProperty)).toBe(false);

    const extraServiceProperty = signedReceipt(receipt('1.0')) as SignedReceipt & {
      receipt: Receipt & { service: Receipt['service'] & { status?: string } };
    };
    extraServiceProperty.receipt.service.status = 'refunded';
    expect(validateReceiptFormat(extraServiceProperty)).toBe(false);
  });

  it('binds a v2 receipt to the exact browser authorization', () => {
    const signed = signedReceipt(receipt('2.0'));
    const authorization: PaymentContextV2 = {
      authorizationVersion: 2,
      recipient: signed.receipt.payment.recipient,
      token: signed.receipt.payment.token,
      amount: signed.receipt.payment.amount,
      nonce: signed.receipt.payment.nonce,
      chainId: signed.receipt.payment.chainId,
      timestamp: signed.receipt.payment.timestamp!,
      audience: signed.receipt.service.audience!,
      method: signed.receipt.service.method!,
      resource: signed.receipt.service.resource!,
      contentType: signed.receipt.service.content_type!,
      requestHash: signed.receipt.service.authorization_request_hash!,
    };

    expect(receiptMatchesPaymentAuthorization(signed, authorization, signed.receipt.payment.payer)).toBe(true);
    expect(
      receiptMatchesPaymentAuthorization(
        signed,
        { ...authorization, timestamp: authorization.timestamp + 1 },
        signed.receipt.payment.payer,
      ),
    ).toBe(false);
  });
});
