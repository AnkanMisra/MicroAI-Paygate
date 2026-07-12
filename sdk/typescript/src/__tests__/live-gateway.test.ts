import { describe, expect, it } from "bun:test";
import { ethers } from "ethers";
import { PaygateClient } from "../index";

const maybeDescribe = process.env.PAYGATE_SDK_LIVE_TEST === "1" ? describe : describe.skip;

maybeDescribe("PaygateClient live gateway flow", () => {
  it("runs against a local gateway started by bun run stack", async () => {
    const privateKey = process.env.EVM_PRIVATE_KEY;
    if (!privateKey) {
      throw new Error("Set EVM_PRIVATE_KEY to an unfunded local/test wallet private key.");
    }
    const trustedServerPublicKey = process.env.PAYGATE_SERVER_PUBLIC_KEY;
    if (!trustedServerPublicKey) {
      throw new Error("Set PAYGATE_SERVER_PUBLIC_KEY to the gateway receipt signing public key.");
    }

    const client = new PaygateClient({
      gatewayUrl: process.env.PAYGATE_GATEWAY_URL ?? "http://localhost:3000",
      signer: new ethers.Wallet(privateKey),
      trustedServerPublicKey,
    });

    const response = await client.summarize("Live SDK test text.");

    expect(response.status).toBe(200);
    expect(response.data.result.length).toBeGreaterThan(0);
    expect(response.receipt?.receipt.id.startsWith("rcpt_")).toBe(true);
    expect(response.receiptVerified).toBe(true);
  }, 30000);
});
