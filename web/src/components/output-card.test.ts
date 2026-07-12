import { describe, expect, test } from "bun:test";
import type { SignedReceipt } from "@/lib/verify-receipt";
import { getReceiptVerificationKey } from "./output-card";

function makeReceipt(overrides: Partial<SignedReceipt> = {}): SignedReceipt {
  return {
    receipt: {
      id: "rcpt_123",
      version: "1",
      timestamp: "2026-06-13T00:00:00Z",
      payment: {
        payer: "0xpayer",
        recipient: "0xrecipient",
        amount: "1",
        token: "USDC",
        chainId: 84532,
        nonce: "nonce-1",
      },
      service: {
        endpoint: "/api/ai/summarize",
        request_hash: "req-hash",
        response_hash: "res-hash",
      },
    },
    signature: "0xsig-a",
    server_public_key: "0xpub-a",
    ...overrides,
  };
}

describe("getReceiptVerificationKey", () => {
  test("returns null when no receipt exists", () => {
    expect(getReceiptVerificationKey(null)).toBeNull();
  });

  test("changes when signed receipt data changes even if receipt id stays the same", () => {
    const first = makeReceipt();
    const second = makeReceipt({ signature: "0xsig-b" });

    expect(first.receipt.id).toBe(second.receipt.id);
    expect(getReceiptVerificationKey(first)).not.toBe(getReceiptVerificationKey(second));
  });
});
