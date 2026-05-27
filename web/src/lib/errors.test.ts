import { describe, expect, it } from "bun:test";
import { classifyError } from "./errors";

// ── HTTP status + body based classification ──────────

describe("classifyError via HTTP context", () => {
  it("classifies 400 + chain_id_mismatch as wrong-chain", () => {
    const e = classifyError(null, {
      status: 400,
      bodyText: '{"error":"chain_id_mismatch"}',
    });
    expect(e.kind).toBe("wrong-chain");
    expect(e.title).toBe("Wrong network");
    expect(e.recoverable).toBe(true);
  });

  it("classifies 400 + invalid_timestamp as expired", () => {
    const e = classifyError(null, {
      status: 400,
      bodyText: '{"error":"invalid_timestamp"}',
    });
    expect(e.kind).toBe("expired");
    expect(e.title).toBe("Payment context expired");
  });

  it("classifies 400 + unknown error as invalid-signature", () => {
    const e = classifyError(null, {
      status: 400,
      bodyText: '{"error":"something_else"}',
    });
    expect(e.kind).toBe("invalid-signature");
  });

  it("classifies 402 as expired", () => {
    const e = classifyError(null, { status: 402 });
    expect(e.kind).toBe("expired");
  });

  it("classifies 403 as invalid-signature", () => {
    const e = classifyError(null, {
      status: 403,
      bodyText: '{"error":"invalid_signature"}',
    });
    expect(e.kind).toBe("invalid-signature");
    expect(e.title).toBe("Signature was rejected");
  });

  it("classifies 408 as ai-timeout", () => {
    const e = classifyError(null, { status: 408 });
    expect(e.kind).toBe("ai-timeout");
  });

  it("classifies 409 + nonce_already_used as duplicate-nonce", () => {
    const e = classifyError(null, {
      status: 409,
      bodyText: '{"error":"nonce_already_used"}',
    });
    expect(e.kind).toBe("duplicate-nonce");
    expect(e.title).toBe("Signature already used");
    expect(e.message).toContain("already submitted");
  });

  it("classifies 409 + other body as invalid-signature", () => {
    const e = classifyError(null, {
      status: 409,
      bodyText: '{"error":"conflict"}',
    });
    expect(e.kind).toBe("invalid-signature");
  });

  it("classifies 429 as rate-limited", () => {
    const e = classifyError(null, { status: 429 });
    expect(e.kind).toBe("rate-limited");
    expect(e.title).toBe("Rate limited");
  });

  it("classifies 502 + verification_unavailable as verifier-unavailable", () => {
    const e = classifyError(null, {
      status: 502,
      bodyText: '{"error":"verification_unavailable"}',
    });
    expect(e.kind).toBe("verifier-unavailable");
    expect(e.title).toBe("Verifier unavailable");
  });

  it("classifies 502 + upstream_unavailable as ai-unavailable", () => {
    const e = classifyError(null, {
      status: 502,
      bodyText: '{"error":"upstream_unavailable"}',
    });
    expect(e.kind).toBe("ai-unavailable");
    expect(e.title).toBe("AI provider unavailable");
  });

  it("classifies 504 + verifier_timeout as verifier-timeout", () => {
    const e = classifyError(null, {
      status: 504,
      bodyText: '{"error":"verifier_timeout"}',
    });
    expect(e.kind).toBe("verifier-timeout");
    expect(e.title).toBe("Verifier timed out");
  });

  it("classifies 504 + upstream_timeout as ai-timeout", () => {
    const e = classifyError(null, {
      status: 504,
      bodyText: '{"error":"upstream_timeout"}',
    });
    expect(e.kind).toBe("ai-timeout");
    expect(e.title).toBe("AI provider timed out");
  });

  it("classifies 5xx unknown as ai-unavailable", () => {
    const e = classifyError(null, { status: 503 });
    expect(e.kind).toBe("ai-unavailable");
  });

  it("classifies unknown status as unknown", () => {
    const e = classifyError(null, { status: 418 });
    expect(e.kind).toBe("unknown");
  });
});

// ── Error object based classification ───────────────

describe("classifyError via thrown Error", () => {
  it("classifies user rejection by code 4001", () => {
    const e = classifyError({ code: 4001, message: "User rejected" });
    expect(e.kind).toBe("user-rejected");
    expect(e.title).toBe("You cancelled the signature");
    expect(e.recoverable).toBe(true);
  });

  it("classifies user rejection by ACTION_REJECTED", () => {
    const e = classifyError({ code: "ACTION_REJECTED", message: "MetaMask rejection" });
    expect(e.kind).toBe("user-rejected");
  });

  it("classifies user rejection by message text", () => {
    const e = classifyError(new Error("user rejected the request"));
    expect(e.kind).toBe("user-rejected");
  });

  it("classifies wrong chain by message text", () => {
    const e = classifyError(new Error("Wrong network. Please switch to Base Sepolia"));
    expect(e.kind).toBe("wrong-chain");
  });

  it("classifies wrong chain by 'did not switch' message", () => {
    const e = classifyError(new Error("Wallet did not switch to chain 84532"));
    expect(e.kind).toBe("wrong-chain");
  });

  it("classifies missing wallet", () => {
    const e = classifyError(new Error("No crypto wallet found"));
    expect(e.kind).toBe("no-wallet");
    expect(e.recoverable).toBe(false);
  });

  it("classifies network fetch error", () => {
    const e = classifyError(new TypeError("Failed to fetch"));
    expect(e.kind).toBe("network");
  });

  it("classifies network error by message", () => {
    const e = classifyError(new Error("NetworkError: connection refused"));
    expect(e.kind).toBe("network");
  });

  it("classifies unknown error as unknown", () => {
    const e = classifyError(new Error("Something unexpected happened"));
    expect(e.kind).toBe("unknown");
    expect(e.title).toBe("Something broke");
  });

  it("handles non-object err gracefully", () => {
    const e = classifyError("string error");
    expect(e.kind).toBe("unknown");
  });

  it("handles null err gracefully", () => {
    const e = classifyError(null);
    expect(e.kind).toBe("unknown");
  });
});

// ── Detail sanitization ─────────────────────────────

describe("sanitizeDetail via classified errors", () => {
  it("extracts clean error code from gateway JSON", () => {
    const e = classifyError(null, {
      status: 403,
      bodyText: '{"error":"invalid_signature","correlation_id":"rc_abc123"}',
    });
    expect(e.detail).toBe("[invalid_signature] correlation_id=rc_abc123");
  });

  it("extracts error code without correlation_id", () => {
    const e = classifyError(null, {
      status: 409,
      bodyText: '{"error":"nonce_already_used"}',
    });
    expect(e.detail).toBe("[nonce_already_used]");
  });

  it("passes through non-JSON body text unchanged", () => {
    const e = classifyError(null, {
      status: 429,
      bodyText: "rate limit exceeded",
    });
    expect(e.detail).toBe("rate limit exceeded");
  });

  it("truncates long non-JSON body", () => {
    const long = "x".repeat(300);
    const e = classifyError(null, { status: 500, bodyText: long });
    expect(e.detail).toBe("x".repeat(200) + "...");
    expect(e.detail!.length).toBe(203);
  });

  it("sets undefined detail when no bodyText", () => {
    const e = classifyError(null, { status: 429 });
    expect(e.detail).toBeUndefined();
  });

  it("uses error.message as detail for thrown errors", () => {
    const e = classifyError(new Error("user rejected the request"));
    expect(e.detail).toBe("user rejected the request");
  });
});
