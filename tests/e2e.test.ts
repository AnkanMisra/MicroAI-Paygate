import { describe, it, expect, beforeAll } from "bun:test";
import { ethers } from "ethers";

const GATEWAY_URL = "http://localhost:3000";
const VERIFIER_URL = "http://localhost:3002";

// Mock wallet for testing
const PRIVATE_KEY = "0x0123456789012345678901234567890123456789012345678901234567890123";
const wallet = new ethers.Wallet(PRIVATE_KEY);

async function signPaymentContext(paymentContext: any) {
  const domain = {
    name: "MicroAI Paygate",
    version: "1",
    chainId: paymentContext.chainId,
    verifyingContract: ethers.ZeroAddress,
  };

  const types = {
    Payment: [
      { name: "recipient", type: "address" },
      { name: "token", type: "string" },
      { name: "amount", type: "string" },
      { name: "nonce", type: "string" },
      { name: "timestamp", type: "uint256" },
    ],
  };

  return wallet.signTypedData(domain, types, {
    recipient: paymentContext.recipient,
    token: paymentContext.token,
    amount: paymentContext.amount,
    nonce: paymentContext.nonce,
    timestamp: paymentContext.timestamp,
  });
}

describe("MicroAI Paygate E2E Flow", () => {
  it("should return 402 Payment Required initially", async () => {
    const res = await fetch(`${GATEWAY_URL}/api/ai/summarize`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: "Hello world" }),
    });

    expect(res.status).toBe(402);
    const data = await res.json() as any;
    expect(data.error).toBe("Payment Required");
    expect(data.paymentContext).toBeDefined();
    expect(data.paymentContext.nonce).toBeDefined();
  });

  it("should accept a valid signature and return result", async () => {
    // 1. Get Nonce
    const initRes = await fetch(`${GATEWAY_URL}/api/ai/summarize`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: "Hello world" }),
    });
    const initData = await initRes.json() as any;
    const { paymentContext } = initData;

    // 2. Sign Data
    const signature = await signPaymentContext(paymentContext);

    // 3. Send Signed Request
    const res = await fetch(`${GATEWAY_URL}/api/ai/summarize`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-402-Signature": signature,
        "X-402-Nonce": paymentContext.nonce,
        "X-402-Timestamp": paymentContext.timestamp.toString(),
      },
      body: JSON.stringify({ text: "This is a test text to summarize." }),
    });

    expect(res.status).toBe(200);
    const data = await res.json() as any;
    expect(data.result).toStartWith("Mock summary:");
    expect(res.headers.get("X-402-Receipt")).toBeTruthy();
  }, 30000);

  it("should reject replayed signed payment context", async () => {
    const initRes = await fetch(`${GATEWAY_URL}/api/ai/summarize`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: "Replay setup" }),
    });
    const initData = await initRes.json() as any;
    const { paymentContext } = initData;
    const signature = await signPaymentContext(paymentContext);
    const body = JSON.stringify({ text: "Replay protection test text." });
    const headers = {
      "Content-Type": "application/json",
      "X-402-Signature": signature,
      "X-402-Nonce": paymentContext.nonce,
      "X-402-Timestamp": paymentContext.timestamp.toString(),
    };

    const first = await fetch(`${GATEWAY_URL}/api/ai/summarize`, {
      method: "POST",
      headers,
      body,
    });

    expect(first.status).toBe(200);

    const second = await fetch(`${GATEWAY_URL}/api/ai/summarize`, {
      method: "POST",
      headers,
      body,
    });

    expect(second.status).toBe(409);
    const data = await second.json() as any;
    expect(data.error).toBe("nonce_already_used");
  }, 30000);
});
