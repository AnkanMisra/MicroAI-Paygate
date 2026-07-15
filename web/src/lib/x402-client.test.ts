import { afterEach, describe, expect, it, mock } from "bun:test";
import {
  buildSignedHeaders,
  getGatewayUrl,
  postSummarize,
  readPaymentChallenge,
  readSummarizeSuccess,
  safeDecodeReceiptHeader,
} from "./x402-client";
import type { PaymentContext } from "./types";
import type { SignedReceipt } from "./verify-receipt";

const originalGatewayUrl = process.env.NEXT_PUBLIC_GATEWAY_URL;
const originalFetch = globalThis.fetch;

const paymentContext: PaymentContext = {
  recipient: "0xrecipient",
  token: "USDC",
  amount: "0.001",
  nonce: "nonce-1",
  chainId: 84532,
  timestamp: 1766611200,
};

const receipt: SignedReceipt = {
  receipt: {
    id: "rcpt_web_1",
    version: "1",
    timestamp: "2026-06-19T00:00:00Z",
    payment: {
      payer: "0xpayer",
      recipient: "0xrecipient",
      amount: "0.001",
      token: "USDC",
      chainId: 84532,
      nonce: "nonce-1",
    },
    service: {
      endpoint: "/api/ai/summarize",
      request_hash: "sha256:request",
      response_hash: "sha256:response",
    },
  },
  signature: "0xsigned",
  server_public_key: "0xserver",
};

function encodeReceipt(value: unknown): string {
  return btoa(JSON.stringify(value));
}

afterEach(() => {
  globalThis.fetch = originalFetch;
  if (originalGatewayUrl === undefined) {
    delete process.env.NEXT_PUBLIC_GATEWAY_URL;
  } else {
    process.env.NEXT_PUBLIC_GATEWAY_URL = originalGatewayUrl;
  }
});

describe("x402 client helpers", () => {
  it("uses the configured gateway URL and localhost fallback", () => {
    delete process.env.NEXT_PUBLIC_GATEWAY_URL;
    expect(getGatewayUrl()).toBe("http://localhost:3000");

    process.env.NEXT_PUBLIC_GATEWAY_URL = "https://gateway.example/";
    expect(getGatewayUrl()).toBe("https://gateway.example/");
  });

  it("builds the summarize request while preserving caller headers", async () => {
    process.env.NEXT_PUBLIC_GATEWAY_URL = "https://gateway.example";
    const calls: Array<{ input: string; init?: RequestInit }> = [];
    const fetchMock = mock((input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ input: String(input), init });
      return Promise.resolve(new Response("ok", { status: 200 }));
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await postSummarize("summarize this", { Authorization: "Bearer test" });

    expect(calls).toHaveLength(1);
    expect(calls[0]?.input).toBe("https://gateway.example/api/ai/summarize");
    expect(calls[0]?.init).toMatchObject({
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer test",
      },
      body: JSON.stringify({ text: "summarize this" }),
    });
  });

  it("reads a payment challenge and rejects a missing context", async () => {
    await expect(
      readPaymentChallenge(
        new Response(JSON.stringify({ paymentContext }), { status: 402 }),
      ),
    ).resolves.toEqual(paymentContext);

    await expect(
      readPaymentChallenge(new Response(JSON.stringify({ error: "missing" }), { status: 402 })),
    ).rejects.toThrow("402 response missing paymentContext");
  });

  it("builds the exact signed retry headers", () => {
    expect(buildSignedHeaders(paymentContext, "0xsignature")).toEqual({
      "X-402-Signature": "0xsignature",
      "X-402-Nonce": "nonce-1",
      "X-402-Timestamp": "1766611200",
    });
  });

  it("keeps a successful summary with a valid receipt header", async () => {
    const result = await readSummarizeSuccess(
      new Response(JSON.stringify({ result: "summary text" }), {
        status: 200,
        headers: { "X-402-Receipt": encodeReceipt(receipt) },
      }),
    );

    expect(result.summary).toBe("summary text");
    expect(result.receipt).toEqual(receipt);
  });

  it("keeps a successful summary when the receipt header is absent", async () => {
    const result = await readSummarizeSuccess(
      new Response(JSON.stringify({ result: "summary without receipt" }), { status: 200 }),
    );

    expect(result).toEqual({ summary: "summary without receipt", receipt: null });
  });

  it.each([
    "not-base64",
    encodeReceipt({ receipt: { id: "rcpt_incomplete" } }),
    encodeReceipt({ ...receipt, receipt: { ...receipt.receipt, payment: { amount: 1 } } }),
  ])("drops malformed receipt header %s without throwing", (header) => {
    expect(safeDecodeReceiptHeader(header)).toBeNull();
  });
});
