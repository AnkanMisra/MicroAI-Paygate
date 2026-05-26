import { ethers } from "ethers";
import { PaygateClient } from "../src";

const gatewayUrl = process.env.PAYGATE_GATEWAY_URL ?? "http://localhost:3000";
const privateKey = process.env.EVM_PRIVATE_KEY;
const trustedServerPublicKey = process.env.PAYGATE_SERVER_PUBLIC_KEY;

if (!privateKey) {
  throw new Error("Set EVM_PRIVATE_KEY to an unfunded local or test wallet private key.");
}

const text = process.argv.slice(2).join(" ") || "Summarize MicroAI Paygate in one sentence.";
const signer = new ethers.Wallet(privateKey);
const client = new PaygateClient({ gatewayUrl, signer, trustedServerPublicKey });

const response = await client.summarize(text);

console.log("Summary:", response.data.result);
console.log("Receipt ID:", response.receipt?.receipt.id ?? "none");
console.log("Receipt verified:", response.receiptVerified);
