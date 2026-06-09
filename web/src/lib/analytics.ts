export type AnalyticsScalar = string | number | boolean | null | undefined;
export type AnalyticsProperties = Record<string, AnalyticsScalar | object>;
export type SanitizedAnalyticsScalar = Exclude<AnalyticsScalar, undefined>;
export type SanitizedAnalyticsProperties = Record<string, SanitizedAnalyticsScalar>;

export type AnalyticsSink = {
  capture: (event: string, properties?: SanitizedAnalyticsProperties) => void;
  identify: (distinctId: string, properties?: SanitizedAnalyticsProperties) => void;
  reset?: () => void;
};

export type AnalyticsClient = {
  capture: (event: string, properties?: AnalyticsProperties) => void;
  identifyWallet: (walletAddress: string, properties?: AnalyticsProperties) => void;
  reset: () => void;
  /**
   * Reconcile the persisted analytics identity against the live wallet. Resets
   * identity when the previously identified wallet is gone or has changed —
   * including across page reloads, where in-memory state is lost but the
   * persisted identity survives.
   */
  reconcileIdentity: (liveWalletAddress: string | null) => void;
};

/**
 * Durable record of which wallet the analytics layer last identified. Backed by
 * `localStorage` in the browser so identity survives reloads; a no-op by default
 * so the pure layer stays testable and disabled mode touches no storage.
 */
export type IdentityStore = {
  get: () => string | null;
  set: (walletAddress: string) => void;
  clear: () => void;
};

const noopIdentityStore: IdentityStore = {
  get: () => null,
  set: () => undefined,
  clear: () => undefined,
};

export type FlowContext = {
  flowRunId: string;
  correlationId: string;
  inputWordCount: number;
  inputCharCount: number;
};

type AnalyticsConfig = {
  enabled: boolean;
  identityStore?: IdentityStore;
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

/**
 * Produce a sanitized properties object containing only allowed scalar values and no blocked keys.
 *
 * @param properties - Input analytics properties which may include objects, undefined values, or blocked keys
 * @returns A new object with the same keys limited to `string | number | boolean | null` values; keys listed in `BLOCKED_PROPERTY_KEYS`, keys with `undefined` values, and non-scalar values are omitted
 */
export function sanitizeAnalyticsProperties(
  properties: AnalyticsProperties = {},
): SanitizedAnalyticsProperties {
  const sanitized: SanitizedAnalyticsProperties = {};

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

/**
 * Builds a FlowContext containing generated IDs and input length metrics for the provided text.
 *
 * @param text - The input text to measure.
 * @param createId - Optional factory to generate unique identifiers; invoked twice to produce `flowRunId` and `correlationId`.
 * @returns A FlowContext with `flowRunId`, `correlationId`, `inputWordCount` (number of whitespace-separated tokens, 0 if the trimmed text is empty), and `inputCharCount` (the original `text.length`).
 */
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

/**
 * Create an AnalyticsClient that forwards events and identity calls to the provided sink when enabled.
 *
 * The client’s `capture`, `identifyWallet`, `reset`, and `reconcileIdentity` methods become no-ops if
 * `config.enabled` is false or `sink` is null. When enabled, `identifyWallet` persists the identified
 * wallet to `config.identityStore` and `reset` clears it, so `reconcileIdentity` can detect a stale
 * identity (e.g. after a reload with a disconnected or switched wallet) that in-memory state alone misses.
 *
 * @param sink - The analytics sink to forward calls to, or `null` to disable forwarding.
 * @param config - Configuration; forwarding occurs only when `config.enabled` is true. `identityStore`
 *   defaults to a no-op store.
 * @returns An AnalyticsClient whose methods forward sanitized properties to `sink` when enabled.
 */
export function createAnalytics(
  sink: AnalyticsSink | null,
  config: AnalyticsConfig,
): AnalyticsClient {
  const enabled = config.enabled && !!sink;
  const identityStore = config.identityStore ?? noopIdentityStore;

  return {
    capture(event, properties) {
      if (!enabled) return;
      invokeSafely(() => sink.capture(event, sanitizeAnalyticsProperties(properties)));
    },
    identifyWallet(walletAddress, properties) {
      if (!enabled) return;
      invokeSafely(() => identityStore.set(walletAddress));
      invokeSafely(() => sink.identify(walletAddress, sanitizeAnalyticsProperties(properties)));
    },
    reset() {
      if (!enabled) return;
      invokeSafely(() => identityStore.clear());
      invokeSafely(() => sink.reset?.());
    },
    reconcileIdentity(liveWalletAddress) {
      if (!enabled) return;
      const persisted = invokeWithFallback(() => identityStore.get(), null);
      if (!persisted) return;
      const live = liveWalletAddress ? liveWalletAddress.toLowerCase() : null;
      if (live === persisted.toLowerCase()) return;
      invokeSafely(() => identityStore.clear());
      invokeSafely(() => sink.reset?.());
    },
  };
}

function invokeWithFallback<T>(fn: () => T, fallback: T): T {
  try {
    return fn();
  } catch (err) {
    console.warn("analytics: non-fatal sink failure", err);
    return fallback;
  }
}

function invokeSafely(fn: () => void): void {
  try {
    fn();
  } catch (err) {
    console.warn("analytics: non-fatal sink failure", err);
  }
}

/**
 * Generate a short unique identifier string.
 *
 * Uses `crypto.randomUUID()` when available; otherwise returns a pseudo-random base-36 string.
 *
 * @returns A unique identifier string.
 */
function defaultCreateId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return Math.random().toString(36).slice(2);
}
