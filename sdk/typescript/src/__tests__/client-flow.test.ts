import { describe, expect, it } from "bun:test";
import { ethers } from "ethers";
import fixture from "../__fixtures__/gateway-receipt.json";
import authorizationV2Fixture from "../../../../tests/fixtures/payment-authorization-v2.json";
import {
  MicroAIPaygateProtocol,
  PaygateClient,
  type PaygateProtocolAdapter,
  type PaymentContext,
  type PaymentContextV2,
  type PaymentRequestBinding,
  type Receipt,
  type SignedReceipt,
} from "../index";

const wallet = new ethers.Wallet(
  "0x0123456789012345678901234567890123456789012345678901234567890123",
);

const paymentContext: PaymentContext = {
  ...(authorizationV2Fixture.context as PaymentContextV2),
  nonce: "client-flow-nonce",
  timestamp: 1766611200,
  audience: "http://gateway.test",
  resource: "/api/ai/summarize",
  requestHash: ethers.sha256(ethers.toUtf8Bytes(JSON.stringify({ text: "hello" }))),
};

const legacyPaymentContext: PaymentContext = {
  recipient: paymentContext.recipient,
  token: paymentContext.token,
  amount: paymentContext.amount,
  nonce: "legacy-client-flow-nonce",
  chainId: paymentContext.chainId,
  timestamp: paymentContext.timestamp,
};

const receiptSigningKey = new ethers.SigningKey(
  "0xabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
);
const trustedServerPublicKey = receiptSigningKey.publicKey;

function sha256Body(bodyText: string): string {
  return `sha256:${ethers.sha256(ethers.toUtf8Bytes(bodyText)).slice(2)}`;
}

function serializeReceiptForGateway(receipt: Receipt): string {
  const service = receipt.version === "1.0"
    ? {
        endpoint: receipt.service.endpoint,
        request_hash: receipt.service.request_hash,
        response_hash: receipt.service.response_hash,
      }
    : receipt.service;
  return JSON.stringify({
    id: receipt.id,
    version: receipt.version,
    timestamp: receipt.timestamp,
    payment: {
      payer: receipt.payment.payer,
      recipient: receipt.payment.recipient,
      amount: receipt.payment.amount,
      token: receipt.payment.token,
      chainId: receipt.payment.chainId,
      nonce: receipt.payment.nonce,
      ...(receipt.version === "2.0" && { timestamp: receipt.payment.timestamp }),
    },
    service,
  }).replace(/[<>&\u2028\u2029]/g, (character) =>
    `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
}

function signedReceiptForPayloads({
  requestBody,
  responseBody,
  endpoint = "/api/ai/summarize",
  authorization = paymentContext as PaymentContextV2,
}: {
  requestBody: string;
  responseBody: string;
  endpoint?: string;
  authorization?: PaymentContextV2;
}): SignedReceipt {
  const receipt: Receipt = {
    id: "rcpt_clientflow1",
    version: "2.0",
    timestamp: "2026-05-25T00:00:00Z",
    payment: {
      payer: wallet.address,
      recipient: authorization.recipient,
      amount: authorization.amount,
      token: authorization.token,
      chainId: authorization.chainId,
      nonce: authorization.nonce,
      timestamp: authorization.timestamp,
    },
    service: {
      endpoint,
      authorization_version: authorization.authorizationVersion,
      audience: authorization.audience,
      method: authorization.method,
      resource: authorization.resource,
      content_type: authorization.contentType,
      authorization_request_hash: authorization.requestHash,
      request_hash: sha256Body(requestBody),
      response_hash: sha256Body(responseBody),
    },
  };
  const receiptHash = ethers.keccak256(ethers.toUtf8Bytes(serializeReceiptForGateway(receipt)));
  return {
    receipt,
    signature: receiptSigningKey.sign(receiptHash).serialized,
    server_public_key: trustedServerPublicKey,
  };
}

function receiptHeader(receipt: SignedReceipt = fixture as SignedReceipt): string {
  return Buffer.from(JSON.stringify(receipt), "utf8").toString("base64");
}

function jsonResponse(body: unknown, init: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });
}

function scriptedFetch(responses: Response[]) {
  const calls: Array<{ input: RequestInfo | URL; init?: RequestInit }> = [];
  const fetcher = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    calls.push({ input, init });
    const response = responses.shift();
    if (!response) {
      throw new Error("unexpected fetch call");
    }
    return response;
  };
  return { calls, fetcher };
}

describe("PaygateClient request flow", () => {
  it("validates a v2 challenge against the exact outgoing request before signing", async () => {
    const context = {
      ...(authorizationV2Fixture.context as PaymentContextV2),
      nonce: "client-v2-binding",
      audience: "http://gateway.test",
      resource: "/api/ai/summarize?mode=brief",
      requestHash: ethers.sha256(ethers.toUtf8Bytes(JSON.stringify({ text: "hello" }))),
    };
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ paymentContext: context }, { status: 402 }),
      jsonResponse({ result: "bound" }, { status: 200 }),
    ]);
    const client = new PaygateClient({ gatewayUrl: "http://gateway.test", signer: wallet, fetch: fetcher });

    await client.request({
      method: "POST",
      path: "/api/ai/summarize?mode=brief",
      body: { text: "hello" },
    });

    expect(calls).toHaveLength(2);
    expect(calls[1].init?.body).toBe(calls[0].init?.body);
    expect(calls[1].init?.headers).toMatchObject({
      "X-402-Payer": wallet.address,
    });
  });

  it("rejects a mismatched v2 request binding before invoking the signer", async () => {
    const context = {
      ...(authorizationV2Fixture.context as PaymentContextV2),
      audience: "http://gateway.test",
      resource: "/api/ai/summarize",
    };
    let signCalls = 0;
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ paymentContext: context }, { status: 402 }),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: {
        async signTypedData() {
          signCalls += 1;
          return "0xnever";
        },
      },
      fetch: fetcher,
    });

    await expect(
      client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "tampered" } }),
    ).rejects.toMatchObject({ code: "payment_binding_mismatch" });
    expect(signCalls).toBe(0);
    expect(calls).toHaveLength(1);
  });

  it("rejects a mismatched v2 binding even when a custom adapter validation is a no-op", async () => {
    const context = {
      ...(authorizationV2Fixture.context as PaymentContextV2),
      audience: "http://gateway.test",
      resource: "/api/ai/summarize",
    };
    class NoOpValidationProtocol extends MicroAIPaygateProtocol {
      signCalls = 0;

      override validatePaymentContext(
        _ctx: PaymentContext,
        _request: PaymentRequestBinding,
      ): void {}

      override signPaymentContext(
        signer: Parameters<MicroAIPaygateProtocol["signPaymentContext"]>[0],
        ctx: PaymentContext,
        payer?: string,
      ): Promise<string> {
        this.signCalls += 1;
        return super.signPaymentContext(signer, ctx, payer);
      }
    }
    const protocol = new NoOpValidationProtocol();
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ paymentContext: context }, { status: 402 }),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
      protocol,
    });

    await expect(
      client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "tampered" } }),
    ).rejects.toMatchObject({ code: "payment_binding_mismatch" });
    expect(protocol.signCalls).toBe(0);
    expect(calls).toHaveLength(1);
  });

  it("rejects a custom adapter downgrade to a legacy unbound context", async () => {
    class DowngradeProtocol extends MicroAIPaygateProtocol {
      signCalls = 0;

      override async readPaymentContext(): Promise<PaymentContext> {
        return legacyPaymentContext;
      }

      override signPaymentContext(
        signer: Parameters<MicroAIPaygateProtocol["signPaymentContext"]>[0],
        ctx: PaymentContext,
        payer?: string,
      ): Promise<string> {
        this.signCalls += 1;
        return super.signPaymentContext(signer, ctx, payer);
      }
    }
    const protocol = new DowngradeProtocol();
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ paymentContext: authorizationV2Fixture.context }, { status: 402 }),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
      protocol,
    });

    await expect(
      client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "hello" } }),
    ).rejects.toMatchObject({ code: "payment_binding_mismatch" });
    expect(protocol.signCalls).toBe(0);
    expect(calls).toHaveLength(1);
  });

  it("handles unsigned request, 402 challenge, signed retry, and verified receipt", async () => {
    const requestBody = JSON.stringify({ text: "hello" });
    const responseBody = JSON.stringify({ result: "summarized text" });
    const receipt = signedReceiptForPayloads({ requestBody, responseBody });
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ error: "Payment Required", paymentContext }, { status: 402 }),
      jsonResponse(
        { result: "summarized text" },
        { status: 200, headers: { "X-402-Receipt": receiptHeader(receipt) } },
      ),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
      trustedServerPublicKey,
    });

    const response = await client.request<{ text: string }, { result: string }>({
      method: "POST",
      path: "/api/ai/summarize",
      body: { text: "hello" },
    });

    expect(response).toMatchObject({
      data: { result: "summarized text" },
      receiptVerified: true,
      status: 200,
    });
    expect(response.receipt?.receipt.id).toBe("rcpt_clientflow1");
    expect(calls).toHaveLength(2);
    expect(String(calls[0].input)).toBe("http://gateway.test/api/ai/summarize");
    expect(calls[0].init?.headers).toEqual({ "Content-Type": "application/json" });
    expect(calls[1].init?.headers).toMatchObject({
      "Content-Type": "application/json",
      "X-402-Nonce": paymentContext.nonce,
      "X-402-Timestamp": paymentContext.timestamp.toString(),
    });
    expect(
      (calls[1].init?.headers as Record<string, string>)["X-402-Signature"].startsWith("0x"),
    ).toBe(true);
  });

  it("falls back to signer getAddress for v2 adapters without getPayer", async () => {
    const requestBody = JSON.stringify({ text: "hello" });
    const responseBody = JSON.stringify({ result: "summarized text" });
    const receipt = signedReceiptForPayloads({ requestBody, responseBody });
    const base = new MicroAIPaygateProtocol();
    const protocol: PaygateProtocolAdapter = {
      readPaymentContext: (response) => base.readPaymentContext(response),
      validatePaymentContext: (context, request) => base.validatePaymentContext(context, request),
      signPaymentContext: (signer, context, payer) => base.signPaymentContext(signer, context, payer),
      buildSignedHeaders: (context, signature, payer) => base.buildSignedHeaders(context, signature, payer),
      readReceipt: (response) => base.readReceipt(response),
    };
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ paymentContext }, { status: 402 }),
      jsonResponse(
        { result: "summarized text" },
        { status: 200, headers: { "X-402-Receipt": receiptHeader(receipt) } },
      ),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
      protocol,
      trustedServerPublicKey,
    });

    const response = await client.summarize("hello");

    expect(response.receiptVerified).toBe(true);
    expect(calls[1].init?.headers).toMatchObject({ "X-402-Payer": wallet.address });
  });

  it("throws typed errors for missing paymentContext and non-JSON 402 bodies", async () => {
    for (const firstResponse of [
      jsonResponse({ error: "Payment Required" }, { status: 402 }),
      new Response("not json", { status: 402 }),
    ]) {
      const { fetcher } = scriptedFetch([firstResponse]);
      const client = new PaygateClient({
        gatewayUrl: "http://gateway.test",
        signer: wallet,
        fetch: fetcher,
      });

      await expect(
        client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "hello" } }),
      ).rejects.toMatchObject({
        code: "payment_challenge_missing",
        status: 402,
      });
    }
  });

  it("throws a typed error when a successful gateway response is not JSON", async () => {
    const { fetcher } = scriptedFetch([new Response("<html>bad gateway</html>", { status: 200 })]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
    });

    await expect(
      client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "hello" } }),
    ).rejects.toMatchObject({
      code: "network_error",
      status: 200,
      bodyText: "<html>bad gateway</html>",
    });
  });

  it("wraps request body serialization failures in a typed SDK error before fetching", async () => {
    let calls = 0;
    const circularBody: { text: string; self?: unknown } = { text: "hello" };
    circularBody.self = circularBody;
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: async () => {
        calls += 1;
        throw new Error("fetch should not be called");
      },
    });

    await expect(
      client.request({
        method: "POST",
        path: "/api/ai/summarize",
        body: circularBody,
      }),
    ).rejects.toMatchObject({
      code: "network_error",
    });
    expect(calls).toBe(0);
  });

  it("rejects fractional and unsafe paymentContext numeric fields before signing", async () => {
    const invalidContexts = [
      { ...paymentContext, chainId: 84532.5 },
      { ...paymentContext, chainId: Number.MAX_SAFE_INTEGER + 1 },
      { ...paymentContext, timestamp: 1766611200.5 },
      { ...paymentContext, timestamp: Number.MAX_SAFE_INTEGER + 1 },
      { ...paymentContext, authorizationVersion: 3 },
      { ...authorizationV2Fixture.context, requestHash: undefined },
    ];

    for (const invalidContext of invalidContexts) {
      const { fetcher } = scriptedFetch([
        jsonResponse({ error: "Payment Required", paymentContext: invalidContext }, { status: 402 }),
      ]);
      const client = new PaygateClient({
        gatewayUrl: "http://gateway.test",
        signer: wallet,
        fetch: fetcher,
      });

      await expect(
        client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "hello" } }),
      ).rejects.toMatchObject({
        code: "payment_challenge_missing",
        status: 402,
      });
    }
  });

  it("throws typed errors for failed signed retries and network failures", async () => {
    const failedRetry = scriptedFetch([
      jsonResponse({ paymentContext }, { status: 402 }),
      new Response("Forbidden", { status: 403 }),
    ]);
    const retryClient = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: failedRetry.fetcher,
    });

    await expect(
      retryClient.request({ method: "POST", path: "/api/ai/summarize", body: { text: "hello" } }),
    ).rejects.toMatchObject({
      code: "signed_retry_failed",
      status: 403,
      bodyText: "Forbidden",
    });

    const networkClient = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: async () => {
        throw new Error("socket closed");
      },
    });

    await expect(
      networkClient.request({
        method: "POST",
        path: "/api/ai/summarize",
        body: { text: "hello" },
      }),
    ).rejects.toMatchObject({
      code: "network_error",
    });
  });

  it("returns null receipt state when success has no X-402-Receipt header", async () => {
    const { fetcher } = scriptedFetch([
      jsonResponse({ paymentContext }, { status: 402 }),
      jsonResponse({ result: "no receipt" }, { status: 200 }),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
    });

    const response = await client.summarize("hello");

    expect(response).toEqual({
      data: { result: "no receipt" },
      receipt: null,
      receiptVerified: null,
      status: 200,
    });
  });

  it("rejects receipts whose hashes do not match the actual request and response bodies", async () => {
    const matchingRequestBody = JSON.stringify({ text: "hello" });
    const matchingResponseBody = JSON.stringify({ result: "summarized text" });
    const staleRequestReceipt = signedReceiptForPayloads({
      requestBody: JSON.stringify({ text: "different input" }),
      responseBody: matchingResponseBody,
    });
    const staleResponseReceipt = signedReceiptForPayloads({
      requestBody: matchingRequestBody,
      responseBody: JSON.stringify({ result: "different output" }),
    });

    for (const receipt of [staleRequestReceipt, staleResponseReceipt]) {
      const { fetcher } = scriptedFetch([
        jsonResponse({ paymentContext }, { status: 402 }),
        jsonResponse(
          { result: "summarized text" },
          { status: 200, headers: { "X-402-Receipt": receiptHeader(receipt) } },
        ),
      ]);
      const client = new PaygateClient({
        gatewayUrl: "http://gateway.test",
        signer: wallet,
        fetch: fetcher,
        trustedServerPublicKey,
      });

      await expect(client.summarize("hello")).rejects.toMatchObject({
        code: "receipt_verification_failed",
        status: 200,
      });
    }
  });

  it("rejects a valid receipt from a different v2 payment authorization", async () => {
    const requestBody = JSON.stringify({ text: "hello" });
    const responseBody = JSON.stringify({ result: "summarized text" });
    const substitutedReceipt = signedReceiptForPayloads({
      requestBody,
      responseBody,
      authorization: { ...paymentContext, nonce: "different-payment-nonce" } as PaymentContextV2,
    });
    const { fetcher } = scriptedFetch([
      jsonResponse({ paymentContext }, { status: 402 }),
      jsonResponse(
        { result: "summarized text" },
        { status: 200, headers: { "X-402-Receipt": receiptHeader(substitutedReceipt) } },
      ),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
      trustedServerPublicKey,
    });

    await expect(client.summarize("hello")).rejects.toMatchObject({
      code: "receipt_verification_failed",
      status: 200,
    });
  });

  it("matches receipt endpoints against the request path, not gatewayUrl path prefixes", async () => {
    const requestBody = JSON.stringify({ text: "hello" });
    const responseBody = JSON.stringify({ result: "summarized text" });
    const prefixedContext = {
      ...paymentContext,
      resource: "/paygate/api/ai/summarize",
    };
    const receipt = signedReceiptForPayloads({
      requestBody,
      responseBody,
      authorization: prefixedContext as PaymentContextV2,
    });
    const { fetcher } = scriptedFetch([
      jsonResponse({ paymentContext: prefixedContext }, { status: 402 }),
      jsonResponse(
        { result: "summarized text" },
        { status: 200, headers: { "X-402-Receipt": receiptHeader(receipt) } },
      ),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test/paygate",
      signer: wallet,
      fetch: fetcher,
      trustedServerPublicKey,
    });

    const response = await client.summarize("hello");

    expect(response.receiptVerified).toBe(true);
  });

  it("does not send requests to absolute paths outside the configured gateway", async () => {
    let calls = 0;
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: async () => {
        calls += 1;
        throw new Error("fetch should not be called");
      },
    });

    await expect(
      client.request({
        method: "POST",
        path: "https://attacker.test/api/ai/summarize",
        body: { text: "hello" },
      }),
    ).rejects.toMatchObject({
      code: "network_error",
    });
    expect(calls).toBe(0);
  });

  it("wraps signer failures with payment_signature_failed", async () => {
    const { fetcher } = scriptedFetch([jsonResponse({ paymentContext }, { status: 402 })]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: {
        signTypedData: async () => {
          throw new Error("user rejected");
        },
      },
      fetch: fetcher,
    });

    await expect(
      client.request({ method: "POST", path: "/api/ai/summarize", body: { text: "hello" } }),
    ).rejects.toMatchObject({
      code: "payment_signature_failed",
    });
  });
});
