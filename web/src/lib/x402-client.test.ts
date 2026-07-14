import { describe, expect, it } from "bun:test";
import { ethers } from "ethers";
import authorizationV2Fixture from "../../../tests/fixtures/payment-authorization-v2.json";
import type { PaymentContextV2 } from "./types";
import {
  buildPaymentTypedData,
  getSummarizeUrl,
  readPaymentChallenge,
  serializeSummarizeRequest,
  validatePaymentContextForRequest,
} from "./x402-client";

describe("request-bound payment authorization", () => {
  it("preserves a configured gateway path prefix", () => {
    const originalGatewayUrl = process.env.NEXT_PUBLIC_GATEWAY_URL;
    process.env.NEXT_PUBLIC_GATEWAY_URL = "https://gateway.example/paygate/";
    try {
      expect(getSummarizeUrl()).toBe("https://gateway.example/paygate/api/ai/summarize");
    } finally {
      if (originalGatewayUrl === undefined) {
        delete process.env.NEXT_PUBLIC_GATEWAY_URL;
      } else {
        process.env.NEXT_PUBLIC_GATEWAY_URL = originalGatewayUrl;
      }
    }
  });

  it("matches the shared v2 typed-data fixture", () => {
    const typedData = buildPaymentTypedData(
      authorizationV2Fixture.context as PaymentContextV2,
      authorizationV2Fixture.payer,
    );

    expect(ethers.TypedDataEncoder.hash(typedData.domain, typedData.types, typedData.value)).toBe(
      authorizationV2Fixture.expectedTypedDataDigest,
    );
    expect(
      ethers.verifyTypedData(
        typedData.domain,
        typedData.types,
        typedData.value,
        authorizationV2Fixture.expectedSignature,
      ),
    ).toBe(authorizationV2Fixture.expectedSigner);
  });

  it("validates the exact serialized bytes and rejects a changed body", () => {
    const context = authorizationV2Fixture.context as PaymentContextV2;
    validatePaymentContextForRequest(context, {
      url: `${context.audience}${context.resource}`,
      method: "POST",
      contentType: "application/json",
      bodyText: authorizationV2Fixture.bodyText,
    });

    expect(() =>
      validatePaymentContextForRequest(context, {
        url: `${context.audience}${context.resource}`,
        method: "POST",
        contentType: "application/json",
        bodyText: serializeSummarizeRequest("changed"),
      }),
    ).toThrow("requestHash");
  });

  it("rejects unknown, partial, and malformed authorization versions", async () => {
    const invalidContexts = [
      { ...authorizationV2Fixture.context, authorizationVersion: 3 },
      { ...authorizationV2Fixture.context, requestHash: undefined },
      { ...authorizationV2Fixture.context, authorizationVersion: undefined },
    ];

    for (const paymentContext of invalidContexts) {
      await expect(
        readPaymentChallenge(
          new Response(JSON.stringify({ paymentContext }), {
            status: 402,
            headers: { "Content-Type": "application/json" },
          }),
        ),
      ).rejects.toThrow("valid paymentContext");
    }
  });
});
