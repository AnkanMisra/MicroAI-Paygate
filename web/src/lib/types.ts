import type { SignedReceipt } from "./verify-receipt";

export type PaymentContextV1 = {
  authorizationVersion?: never;
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
};

export type PaymentContextV2 = {
  authorizationVersion: 2;
  recipient: string;
  token: string;
  amount: string;
  nonce: string;
  chainId: number;
  timestamp: number;
  audience: string;
  method: string;
  resource: string;
  contentType: string;
  requestHash: string;
};

export type PaymentContext = PaymentContextV1 | PaymentContextV2;

export type X402Step =
  | "idle"
  | "request"
  | "challenge"
  | "sign"
  | "verify"
  | "ai"
  | "receipt"
  | "done"
  | "error";

export const X402_STEPS: readonly Exclude<X402Step, "idle" | "done" | "error">[] = [
  "request",
  "challenge",
  "sign",
  "verify",
  "ai",
  "receipt",
] as const;

export type StepLabel = {
  id: Exclude<X402Step, "idle" | "done" | "error">;
  index: number;
  short: string;
  long: string;
};

export const STEP_LABELS: readonly StepLabel[] = [
  { id: "request", index: 1, short: "Request", long: "Send unsigned request" },
  { id: "challenge", index: 2, short: "Challenge", long: "Receive 402 payment context" },
  { id: "sign", index: 3, short: "Sign", long: "Sign EIP-712 in wallet" },
  { id: "verify", index: 4, short: "Verify", long: "Verifier checks signature" },
  { id: "ai", index: 5, short: "Generate", long: "AI provider runs the request" },
  { id: "receipt", index: 6, short: "Receipt", long: "Signed receipt returned" },
] as const;

export type WalletState =
  | { status: "missing" }
  | { status: "disconnected" }
  | {
      status: "connected";
      address: string;
      chainId: number;
    };

export type StoredReceiptEntry = {
  receipt: SignedReceipt;
  savedAt: number;
  promptPreview: string;
};
