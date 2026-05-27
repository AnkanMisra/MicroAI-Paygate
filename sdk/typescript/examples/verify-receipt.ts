import { decodeReceiptHeader, fetchReceipt, verifyReceipt, type SignedReceipt } from "../src";

const gatewayUrl = process.env.PAYGATE_GATEWAY_URL ?? "http://localhost:3000";
const trustedServerPublicKey = process.env.PAYGATE_SERVER_PUBLIC_KEY;
const receiptInput = process.argv[2];

if (!trustedServerPublicKey) {
  throw new Error("Set PAYGATE_SERVER_PUBLIC_KEY to the trusted gateway receipt signing key.");
}

if (!receiptInput) {
  throw new Error("Pass a receipt ID like rcpt_... or a base64 X-402-Receipt header value.");
}

async function loadReceipt(input: string): Promise<SignedReceipt> {
  if (input.startsWith("rcpt_")) {
    const receipt = await fetchReceipt(input, gatewayUrl);
    if (receipt === null) {
      throw new Error(`Receipt ${input} was not found at ${gatewayUrl}.`);
    }
    return receipt;
  }

  return decodeReceiptHeader(input);
}

const receipt = await loadReceipt(receiptInput);
const verified = await verifyReceipt(receipt, {
  expectedServerPublicKey: trustedServerPublicKey,
});

console.log("Receipt ID:", receipt.receipt.id);
console.log("Payer:", receipt.receipt.payment.payer);
console.log("Endpoint:", receipt.receipt.service.endpoint);
console.log("Receipt verified:", verified);

if (!verified) {
  throw new Error("Receipt signature did not verify against PAYGATE_SERVER_PUBLIC_KEY.");
}
