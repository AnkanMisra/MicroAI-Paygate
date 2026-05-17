export type ErrorKind =
  | "no-wallet"
  | "wrong-chain"
  | "user-rejected"
  | "expired"
  | "invalid-signature"
  | "rate-limited"
  | "ai-timeout"
  | "ai-unavailable"
  | "verifier-timeout"
  | "verifier-unavailable"
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
    message:
      "Your wallet isn't on the expected network. Use the switch button in the wallet widget to fix it.",
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
  "verifier-timeout": {
    title: "Verifier timed out",
    message:
      "The signature verifier didn't respond in time. Your signature wasn't accepted — no payment occurred. Retry.",
  },
  "verifier-unavailable": {
    title: "Verifier unavailable",
    message:
      "The signature verifier is down. Your signature wasn't accepted — no payment occurred. Retry in a moment.",
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

// Maps gateway HTTP status + sanitized error body to a user-facing kind.
// The gateway's public error codes (gateway/errors.go) are stable strings
// like "chain_id_mismatch", "invalid_timestamp", "verifier_timeout",
// "upstream_timeout", "nonce_already_used".
function statusToKind(status: number, body: string): ErrorKind {
  if (status === 400) {
    if (body.includes("chain_id_mismatch")) return "wrong-chain";
    if (body.includes("invalid_timestamp")) return "expired";
    return "invalid-signature";
  }
  if (status === 402) return "expired";
  if (status === 403) return "invalid-signature";
  if (status === 408) return "ai-timeout";
  if (status === 504) {
    // Verifier timeout means we never reached the AI — different copy.
    return body.includes("verifier_timeout") ? "verifier-timeout" : "ai-timeout";
  }
  if (status === 409) {
    return body.includes("nonce_already_used") ? "expired" : "invalid-signature";
  }
  if (status === 429) return "rate-limited";
  if (status === 502) {
    // Gateway returns 502 + verification_unavailable when the verifier didn't
    // respond (gateway/main.go:409,419) — signing never succeeded. The
    // upstream_unavailable case is post-payment AI-provider failure.
    return body.includes("verification_unavailable") ? "verifier-unavailable" : "ai-unavailable";
  }
  if (status >= 500) return "ai-unavailable";
  return "unknown";
}

function looksWrongChain(message: string): boolean {
  const m = message.toLowerCase();
  return (
    m.includes("wrong network") ||
    m.includes("wrong chain") ||
    m.includes("chain mismatch") ||
    m.includes("unsupported chain") ||
    m.includes("chain not supported") ||
    m.includes("incorrect network") ||
    m.includes("did not switch to chain")
  );
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
    if (looksWrongChain(message)) return build("wrong-chain", message);
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
