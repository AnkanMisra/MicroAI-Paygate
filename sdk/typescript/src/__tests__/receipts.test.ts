import { describe, expect, it } from "bun:test";
import { ethers } from "ethers";
import fixture from "../__fixtures__/gateway-receipt.json";
import {
  PaygateSdkError,
  decodeReceiptHeader,
  fetchReceipt,
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

const fixtureServerPublicKey = (fixture as SignedReceipt).server_public_key;

async function expectInvalid(mutator: (receipt: SignedReceipt) => void) {
  const tampered = cloneFixture();
  mutator(tampered);
  expect(await verifyReceipt(tampered, { expectedServerPublicKey: fixtureServerPublicKey })).toBe(
    false,
  );
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
    expect(
      await verifyReceipt(cloneFixture(), { expectedServerPublicKey: fixtureServerPublicKey }),
    ).toBe(true);
  });

  it("verifyReceipt preserves request-bound v2 receipt fields in the signed payload", async () => {
    const signingKey = new ethers.SigningKey(
      "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    );
    const receipt: SignedReceipt["receipt"] = {
      id: "rcpt_boundv2test",
      version: "2.0",
      timestamp: "2026-07-16T00:00:00Z",
      payment: {
        payer: "0x14791697260E4c9A71f18484C9f997B308e59325",
        recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
        amount: "0.001",
        token: "USDC",
        chainId: 84532,
        nonce: "bound-v2-receipt",
        timestamp: 1760572800,
      },
      service: {
        endpoint: "/api/ai/summarize",
        authorization_version: 2,
        audience: "https://gateway.example.com",
        method: "POST",
        resource: "/api/ai/summarize?mode=brief&tag=<x>\u2028",
        content_type: "application/json",
        authorization_request_hash:
          "0x8187d0879ad19b46b277e1b761d3f70d51bc9de6459530b686cfaa503ae8d0e9",
        request_hash:
          "sha256:8187d0879ad19b46b277e1b761d3f70d51bc9de6459530b686cfaa503ae8d0e9",
        response_hash:
          "sha256:8a90fd4352d6e287b3e908e62f802c99c4f5680c9644cb27fb64d638e3fbb9d4",
      },
    };
    const goJSON = JSON.stringify(receipt).replace(/[<>&\u2028\u2029]/g, (character) =>
      `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`,
    );
    const digest = ethers.keccak256(ethers.toUtf8Bytes(goJSON));
    const signature = signingKey.sign(digest);
    const signedReceipt: SignedReceipt = {
      receipt,
      signature: ethers.hexlify(
        ethers.concat([signature.r, signature.s, ethers.toBeHex(signature.yParity, 1)]),
      ),
      server_public_key: signingKey.publicKey,
    };

    expect(
      await verifyReceipt(signedReceipt, { expectedServerPublicKey: signingKey.publicKey }),
    ).toBe(true);
  });

  it("rejects unsupported versions and unsigned v2 metadata on legacy receipts", () => {
    const unsupported = cloneFixture();
    unsupported.receipt.version = "3.0";
    expect(validateReceiptFormat(unsupported)).toBe(false);

    const legacyWithV2Metadata = cloneFixture();
    legacyWithV2Metadata.receipt.service.authorization_version = 2;
    legacyWithV2Metadata.receipt.service.audience = "https://gateway.example.com";
    expect(validateReceiptFormat(legacyWithV2Metadata)).toBe(false);

    const extraProperty = cloneFixture() as SignedReceipt & { receipt: { status?: string } };
    extraProperty.receipt.status = "refunded";
    expect(validateReceiptFormat(extraProperty)).toBe(false);

    const extraServiceProperty = cloneFixture() as SignedReceipt & {
      receipt: { service: SignedReceipt["receipt"]["service"] & { status?: string } };
    };
    extraServiceProperty.receipt.service.status = "refunded";
    expect(validateReceiptFormat(extraServiceProperty)).toBe(false);
  });

  it("verifyReceipt requires the expected gateway receipt signing key as a trust anchor", async () => {
    expect(await verifyReceipt(cloneFixture())).toBe(false);
    expect(
      await verifyReceipt(cloneFixture(), {
        expectedServerPublicKey:
          "0x04a96f0eb0070322ef61fba98b6d289430668734b57a005a327111fc470bdbf9677b20c97fbeac68dd514d6792e21b02737636e30511449d5969722faa29ce7ed4",
      }),
    ).toBe(false);
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

  it("fetchReceipt returns receipts, null for 404, and typed decode failures", async () => {
    const calls: string[] = [];
    const okReceipt = await fetchReceipt("rcpt_sdkfixture1", "http://gateway.test/", async (url) => {
      calls.push(String(url));
      return jsonResponse(fixture, { status: 200 });
    });

    expect(okReceipt?.receipt.id).toBe("rcpt_sdkfixture1");
    expect(calls).toEqual(["http://gateway.test/api/receipts/rcpt_sdkfixture1"]);

    const missingReceipt = await fetchReceipt("rcpt_missing", "http://gateway.test", async () => {
      return jsonResponse({ error: "Receipt not found" }, { status: 404 });
    });
    expect(missingReceipt).toBeNull();

    await expect(
      fetchReceipt("rcpt_bad", "http://gateway.test", async () => {
        return new Response("not json", { status: 200 });
      }),
    ).rejects.toMatchObject({
      code: "receipt_decode_failed",
      status: 200,
    });
  });
});

function jsonResponse(body: unknown, init: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });
}
