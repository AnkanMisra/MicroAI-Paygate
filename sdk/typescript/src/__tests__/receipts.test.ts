import { describe, expect, it } from "bun:test";
import fixture from "../__fixtures__/gateway-receipt.json";
import {
  PaygateSdkError,
  decodeReceiptHeader,
  validateReceiptFormat,
  verifyReceipt,
  type SignedReceipt,
} from "../index";

function encodeHeader(value: unknown): string {
  return Buffer.from(JSON.stringify(value), "utf8").toString("base64");
}

function cloneFixture(): SignedReceipt {
  return structuredClone(fixture) as SignedReceipt;
}

async function expectInvalid(mutator: (receipt: SignedReceipt) => void) {
  const tampered = cloneFixture();
  mutator(tampered);
  expect(await verifyReceipt(tampered)).toBe(false);
}

describe("receipt helpers", () => {
  it("decodeReceiptHeader accepts valid base64 SignedReceipt JSON", () => {
    const decoded = decodeReceiptHeader(encodeHeader(fixture));

    expect(decoded.receipt.id).toBe("rcpt_sdkfixture1");
    expect(decoded.receipt.service.endpoint).toBe("/api/ai/summarize");
  });

  it("decodeReceiptHeader rejects malformed base64, malformed JSON, and wrong receipt shapes", () => {
    for (const header of [
      "@@@not-base64@@@",
      Buffer.from("not json", "utf8").toString("base64"),
      encodeHeader({ receipt: { id: "rcpt_incomplete" } }),
    ]) {
      expect(() => decodeReceiptHeader(header)).toThrow(PaygateSdkError);
      try {
        decodeReceiptHeader(header);
      } catch (error) {
        expect((error as PaygateSdkError).code).toBe("receipt_decode_failed");
      }
    }
  });

  it("validateReceiptFormat checks the gateway SignedReceipt shape without verifying the signature", () => {
    expect(validateReceiptFormat(fixture)).toBe(true);
    expect(validateReceiptFormat({ receipt: { id: "rcpt_incomplete" } })).toBe(false);
    expect(validateReceiptFormat(null)).toBe(false);
  });

  it("verifyReceipt verifies the gateway-format receipt fixture", async () => {
    expect(await verifyReceipt(cloneFixture())).toBe(true);
  });

  it("verifyReceipt returns false for tampered receipt fields and key material", async () => {
    await expectInvalid((receipt) => {
      receipt.receipt.payment.amount = "0.002";
    });
    await expectInvalid((receipt) => {
      receipt.receipt.payment.nonce = "different-nonce";
    });
    await expectInvalid((receipt) => {
      receipt.receipt.service.response_hash =
        "sha256:0000000000000000000000000000000000000000000000000000000000000000";
    });
    await expectInvalid((receipt) => {
      receipt.signature = `${receipt.signature.slice(0, -2)}01`;
    });
    await expectInvalid((receipt) => {
      receipt.server_public_key =
        "0x04a96f0eb0070322ef61fba98b6d289430668734b57a005a327111fc470bdbf9677b20c97fbeac68dd514d6792e21b02737636e30511449d5969722faa29ce7ed4";
    });
  });
});
