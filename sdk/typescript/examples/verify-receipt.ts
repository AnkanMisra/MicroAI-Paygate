import { ethers } from "ethers";
import { PaygateClient, PaygateSdkError } from "../src";

async function main(): Promise<void> {
  const gatewayUrl = process.env.PAYGATE_GATEWAY_URL ?? "http://localhost:3000";
  const privateKey = process.env.EVM_PRIVATE_KEY;
  const trustedServerPublicKey =
    process.env.PAYGATE_SERVER_PUBLIC_KEY ?? "0xYOUR_GATEWAY_PUBLIC_KEY";

  if (!privateKey) {
    throw new Error("Set EVM_PRIVATE_KEY to an unfunded local or test wallet private key.");
  }

  if (trustedServerPublicKey === "0xYOUR_GATEWAY_PUBLIC_KEY") {
    throw new Error("Set PAYGATE_SERVER_PUBLIC_KEY to the trusted gateway public key.");
  }

  const text = process.argv.slice(2).join(" ") || "Summarize MicroAI Paygate in one sentence.";
  const signer = new ethers.Wallet(privateKey);
  const client = new PaygateClient({ gatewayUrl, signer, trustedServerPublicKey });

  try {
    const response = await client.summarize(text);

    if (!response.receipt) {
      console.warn("No receipt returned; treat the response as untrusted.");
      process.exitCode = 1;
      return;
    }

    if (!response.receiptVerified) {
      console.warn("Receipt was present but did not verify; discard the response.");
      process.exitCode = 1;
      return;
    }

    console.log("Summary:", response.data.result);
    console.log("Verified receipt ID:", response.receipt.receipt.id);
  } catch (error) {
    if (
      error instanceof PaygateSdkError &&
      (error.code === "receipt_verification_failed" || error.code === "receipt_decode_failed")
    ) {
      console.error("Receipt verification failed; treat the response as untrusted.");
      process.exitCode = 1;
      return;
    }
    throw error;
  }
}

await main();