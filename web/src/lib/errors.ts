export type ErrorKind =
  | "no-wallet"
  | "wrong-chain"
  | "user-rejected"
  | "expired"
  | "invalid-signature"
  | "rate-limited"
  | "ai-timeout"
  | "ai-unavailable"
  | "network"
  | "unknown";

export type ClassifiedError = {
  kind: ErrorKind;
  title: string;
  message: string;
  detail?: string;
  recoverable: boolean;
};

export type ErrorContext = {
  status?: number;
  bodyText?: string;
};

const COPY: Record<ErrorKind, { title: string; message: string }> = {
  "no-wallet": {
    title: "No wallet detected",
    message: "Install MetaMask, Rabby, or Coinbase Wallet, then refresh this page.",
  },
  "wrong-chain": {
    title: "Wrong network",
    message: "Your wallet isn't on Base Sepolia. Use the switch button above to fix it.",
  },
  "user-rejected": {
    title: "You cancelled the signature",
    message: "No payment was sent. Try again whenever you're ready.",
  },
  expired: {
    title: "Payment context expired",
    message: "The signed challenge took too long to return. Retry to get a fresh one.",
  },
  "invalid-signature": {
    title: "Signature was rejected",
    message:
      "The verifier didn't accept the signature. Double-check your wallet account hasn't changed mid-flow, then retry.",
  },
  "rate-limited": {
    title: "Rate limited",
    message: "Too many requests recently. Wait a moment and try again.",
  },
  "ai-timeout": {
    title: "AI provider timed out",
    message:
      "Payment was verified, but the AI provider didn't respond in time. Your wallet wasn't charged. Retry.",
  },
  "ai-unavailable": {
    title: "AI provider unavailable",
    message: "The upstream model is down right now. Try again in a minute.",
  },
  network: {
    title: "Network error",
    message: "Couldn't reach the gateway. Check your connection and retry.",
  },
  unknown: {
    title: "Something broke",
    message: "An unexpected error happened. Retry — and if it persists, check the console.",
  },
};

function build(kind: ErrorKind, detail?: string): ClassifiedError {
  return {
    kind,
    ...COPY[kind],
    detail,
    recoverable: kind !== "no-wallet",
  };
}

function statusToKind(status: number, body: string): ErrorKind {
  if (status === 402) return "expired";
  if (status === 403) return "invalid-signature";
  if (status === 408 || status === 504) return "ai-timeout";
  if (status === 409) {
    return body.includes("nonce_already_used") ? "expired" : "invalid-signature";
  }
  if (status === 429) return "rate-limited";
  if (status === 502) return "ai-unavailable";
  if (status >= 500) return "ai-unavailable";
  return "unknown";
}

function looksRejected(message: string, code?: number | string): boolean {
  if (code === 4001 || code === "ACTION_REJECTED") return true;
  const m = message.toLowerCase();
  return (
    m.includes("user rejected") ||
    m.includes("user denied") ||
    m.includes("rejected the request") ||
    m.includes("action_rejected")
  );
}

export function classifyError(err: unknown, ctx?: ErrorContext): ClassifiedError {
  if (ctx?.status !== undefined && ctx.status !== 0) {
    return build(statusToKind(ctx.status, ctx.bodyText ?? ""), ctx.bodyText);
  }

  if (typeof err === "object" && err !== null) {
    const e = err as { message?: string; code?: number | string; shortMessage?: string };
    const message = e.shortMessage ?? e.message ?? "";
    if (looksRejected(message, e.code)) return build("user-rejected", message);
    if (message.toLowerCase().includes("no wallet") || message.toLowerCase().includes("no crypto wallet")) {
      return build("no-wallet", message);
    }
    if (message.toLowerCase().includes("failed to fetch") || message.toLowerCase().includes("networkerror")) {
      return build("network", message);
    }
    return build("unknown", message);
  }

  return build("unknown", String(err));
}
