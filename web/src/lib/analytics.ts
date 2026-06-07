export type AnalyticsScalar = string | number | boolean | null | undefined;
export type AnalyticsProperties = Record<string, AnalyticsScalar | object>;

export type AnalyticsSink = {
  capture: (event: string, properties?: Record<string, AnalyticsScalar>) => void;
  identify: (distinctId: string, properties?: Record<string, AnalyticsScalar>) => void;
  reset?: () => void;
};

export type AnalyticsClient = {
  capture: (event: string, properties?: AnalyticsProperties) => void;
  identifyWallet: (walletAddress: string, properties?: AnalyticsProperties) => void;
  reset: () => void;
};

export type FlowContext = {
  flowRunId: string;
  correlationId: string;
  inputWordCount: number;
  inputCharCount: number;
};

type AnalyticsConfig = {
  enabled: boolean;
};

const BLOCKED_PROPERTY_KEYS = new Set([
  "text",
  "prompt",
  "prompt_preview",
  "summary",
  "summary_text",
  "signature",
  "nonce",
  "receipt",
  "receipt_payload",
  "payment_context",
  "request_body",
  "response_body",
]);

export function sanitizeAnalyticsProperties(
  properties: AnalyticsProperties = {},
): Record<string, AnalyticsScalar> {
  const sanitized: Record<string, AnalyticsScalar> = {};

  for (const [key, value] of Object.entries(properties)) {
    if (BLOCKED_PROPERTY_KEYS.has(key)) continue;
    if (value === undefined) continue;
    if (
      value === null ||
      typeof value === "string" ||
      typeof value === "number" ||
      typeof value === "boolean"
    ) {
      sanitized[key] = value;
    }
  }

  return sanitized;
}

export function createFlowContext(
  text: string,
  createId: () => string = defaultCreateId,
): FlowContext {
  const trimmed = text.trim();
  return {
    flowRunId: createId(),
    correlationId: createId(),
    inputWordCount: trimmed ? trimmed.split(/\s+/).length : 0,
    inputCharCount: text.length,
  };
}

export function createAnalytics(
  sink: AnalyticsSink | null,
  config: AnalyticsConfig,
): AnalyticsClient {
  const enabled = config.enabled && !!sink;

  return {
    capture(event, properties) {
      if (!enabled) return;
      sink.capture(event, sanitizeAnalyticsProperties(properties));
    },
    identifyWallet(walletAddress, properties) {
      if (!enabled) return;
      sink.identify(walletAddress, sanitizeAnalyticsProperties(properties));
    },
    reset() {
      if (!enabled) return;
      sink.reset?.();
    },
  };
}

function defaultCreateId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return Math.random().toString(36).slice(2);
}
