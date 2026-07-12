import { beforeEach, describe, expect, it, mock } from "bun:test";
import {
  createAnalytics,
  createFlowContext,
  sanitizeAnalyticsProperties,
  type AnalyticsSink,
} from "./analytics";

describe("sanitizeAnalyticsProperties", () => {
  it("drops raw content and payment artifacts", () => {
    const sanitized = sanitizeAnalyticsProperties({
      text: "raw prompt",
      summary: "raw summary",
      signature: "0xabc",
      nonce: "nonce-123",
      receipt: { id: "rcpt_1" },
      payment_context: { amount: "0.001" },
      input_word_count: 42,
      status_code: 402,
      cache_hit: false,
    });

    expect(sanitized).toEqual({
      input_word_count: 42,
      status_code: 402,
      cache_hit: false,
    });
  });

  it("drops undefined values and preserves scalar metadata", () => {
    const sanitized = sanitizeAnalyticsProperties({
      flow_run_id: "flow_123",
      correlation_id: "corr_123",
      error_kind: undefined,
      has_receipt: true,
      input_char_count: 128,
    });

    expect(sanitized).toEqual({
      flow_run_id: "flow_123",
      correlation_id: "corr_123",
      has_receipt: true,
      input_char_count: 128,
    });
  });
});

describe("createFlowContext", () => {
  it("builds stable flow metadata without retaining raw prompt text", () => {
    let call = 0;
    const makeId = mock(() => (++call === 1 ? "flow-1" : "corr-1"));

    const flow = createFlowContext("hello  world", makeId);

    expect(flow).toEqual({
      flowRunId: "flow-1",
      correlationId: "corr-1",
      inputCharCount: 12,
      inputWordCount: 2,
    });
  });
});

describe("createAnalytics", () => {
  let sink: AnalyticsSink;

  beforeEach(() => {
    sink = {
      capture: mock(() => undefined),
      identify: mock(() => undefined),
      reset: mock(() => undefined),
    };
  });

  it("captures sanitized events when enabled", () => {
    const analytics = createAnalytics(sink, { enabled: true });

    analytics.capture("summary requested", {
      text: "do not keep this",
      flow_run_id: "flow-1",
      correlation_id: "corr-1",
      input_word_count: 33,
    });

    expect(sink.capture).toHaveBeenCalledWith("summary requested", {
      flow_run_id: "flow-1",
      correlation_id: "corr-1",
      input_word_count: 33,
    });
  });

  it("identifies the wallet only after explicit identifyWallet call", () => {
    const analytics = createAnalytics(sink, { enabled: true });

    analytics.capture("wallet connect requested", { flow_run_id: "flow-1" });
    expect(sink.identify).not.toHaveBeenCalled();

    analytics.identifyWallet("0xAbC123", {
      wallet_connected: true,
      chain_id: 84532,
    });

    expect(sink.identify).toHaveBeenCalledWith("0xAbC123", {
      wallet_connected: true,
      chain_id: 84532,
    });
  });

  it("becomes a no-op when disabled", () => {
    const analytics = createAnalytics(sink, { enabled: false });

    analytics.capture("summary requested", { flow_run_id: "flow-1" });
    analytics.identifyWallet("0xAbC123");
    analytics.reset();

    expect(sink.capture).not.toHaveBeenCalled();
    expect(sink.identify).not.toHaveBeenCalled();
    expect(sink.reset).not.toHaveBeenCalled();
  });

  it("swallows sink failures so analytics never breaks product code", () => {
    sink.capture = mock(() => {
      throw new Error("capture broke");
    });
    sink.identify = mock(() => {
      throw new Error("identify broke");
    });
    sink.reset = mock(() => {
      throw new Error("reset broke");
    });

    const analytics = createAnalytics(sink, { enabled: true });

    expect(() => analytics.capture("summary requested", { flow_run_id: "flow-1" })).not.toThrow();
    expect(() => analytics.identifyWallet("0xAbC123")).not.toThrow();
    expect(() => analytics.reset()).not.toThrow();
  });
});
