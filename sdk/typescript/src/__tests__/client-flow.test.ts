import { describe, expect, it } from "bun:test";
import { ethers } from "ethers";
import fixture from "../__fixtures__/gateway-receipt.json";
import {
  PaygateClient,
  type PaymentContext,
  type SignedReceipt,
} from "../index";

const wallet = new ethers.Wallet(
  "0x0123456789012345678901234567890123456789012345678901234567890123",
);

const paymentContext: PaymentContext = {
  recipient: "0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219",
  token: "USDC",
  amount: "0.001",
  nonce: "client-flow-nonce",
  chainId: 84532,
  timestamp: 1766611200,
};

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
  it("handles unsigned request, 402 challenge, signed retry, and verified receipt", async () => {
    const { calls, fetcher } = scriptedFetch([
      jsonResponse({ error: "Payment Required", paymentContext }, { status: 402 }),
      jsonResponse(
        { result: "summarized text" },
        { status: 200, headers: { "X-402-Receipt": receiptHeader() } },
      ),
    ]);
    const client = new PaygateClient({
      gatewayUrl: "http://gateway.test",
      signer: wallet,
      fetch: fetcher,
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
    expect(response.receipt?.receipt.id).toBe("rcpt_sdkfixture1");
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
