import {
  createAnalytics,
  type AnalyticsClient,
  type AnalyticsSink,
  type IdentityStore,
} from "./analytics";

type PostHogClient = (typeof import("posthog-js"))["default"];
type PostHogLoader = () => Promise<{ default: PostHogClient }>;
type RetryScheduler = (retry: () => void, attempt: number) => void;
type BrowserAnalyticsEnv = {
  enabledFlag: string;
  token: string;
  host: string;
  hasWindow: boolean;
};

// Persisted so a wallet identified in one page session can be reconciled (and
// reset) on the next load, where in-memory state is gone but PostHog's
// distinct_id survives in its own storage.
const IDENTITY_STORAGE_KEY = "microai.analytics.identity";
// Bounded so a permanently broken network/SDK can't retry forever.
const MAX_INIT_ATTEMPTS = 3;

let initialized = false;
let initPromise: Promise<void> | null = null;
let posthogClient: PostHogClient | null = null;
let initAttempts = 0;
const pendingOps: Array<(client: PostHogClient) => void> = [];

const identityStore: IdentityStore = {
  get: () => {
    if (typeof window === "undefined") return null;
    return window.localStorage.getItem(IDENTITY_STORAGE_KEY);
  },
  set: (walletAddress) => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(IDENTITY_STORAGE_KEY, walletAddress);
  },
  clear: () => {
    if (typeof window === "undefined") return;
    window.localStorage.removeItem(IDENTITY_STORAGE_KEY);
  },
};

const sink: AnalyticsSink = {
  capture: (event, properties) => {
    withPostHog((client) => client.capture(event, properties));
  },
  identify: (distinctId, properties) => {
    withPostHog((client) => client.identify(distinctId, properties));
  },
  reset: () => {
    withPostHog((client) => client.reset());
  },
};

export const browserAnalytics: AnalyticsClient = createAnalytics(sink, {
  enabled: shouldEnablePostHog(readBrowserAnalyticsEnv()),
  identityStore,
});

/**
 * Initializes the PostHog browser SDK with the configured project token and host using the module's preferred settings.
 *
 * This is a no-op if analytics are disabled or the SDK has already been initialized. When run, it configures PostHog to disable autocapture and session recording, sets pageview/capture behavior, and marks the module as initialized. If the dynamic import or `posthog.init` fails transiently, a bounded auto-retry is scheduled (up to `MAX_INIT_ATTEMPTS`) so a one-shot caller still recovers without a full page reload.
 *
 * @param loadPostHog - Loader for the PostHog module; injectable for tests.
 * @param schedule - Scheduler used to defer a retry after a failed init; injectable for tests. Defaults to a backoff-based `setTimeout`.
 */
export function initBrowserAnalytics(
  loadPostHog: PostHogLoader = () => import("posthog-js"),
  schedule: RetryScheduler = defaultRetryScheduler,
): void {
  const env = readBrowserAnalyticsEnv();
  if (initialized || initPromise || !shouldEnablePostHog(env)) return;

  initPromise = loadPostHog()
    .then(({ default: posthog }) => {
      posthogClient = posthog;
      posthog.init(env.token, {
        api_host: env.host,
        defaults: "2025-05-24",
        autocapture: false,
        capture_pageview: "history_change",
        capture_pageleave: "if_capture_pageview",
        person_profiles: "identified_only",
        disable_session_recording: true,
      });
      initialized = true;
      initAttempts = 0;
      flushPendingOps();
    })
    .catch((err) => {
      console.warn("analytics: failed to initialize PostHog", err);
      initialized = false;
      initPromise = null;
      posthogClient = null;
      pendingOps.length = 0;
      initAttempts += 1;
      if (initAttempts < MAX_INIT_ATTEMPTS) {
        schedule(() => initBrowserAnalytics(loadPostHog, schedule), initAttempts);
      }
    });
}

/**
 * Determine whether PostHog analytics should be enabled in the current environment.
 *
 * @returns `true` only when the enable flag is exactly `"true"` or `"1"` (after trim/lowercase), the project token is non-empty, and code is running in a browser (`window` is defined); `false` otherwise.
 */
export function shouldEnablePostHog(env: BrowserAnalyticsEnv): boolean {
  return (
    (env.enabledFlag === "true" || env.enabledFlag === "1") &&
    env.token.length > 0 &&
    env.hasWindow
  );
}

function readBrowserAnalyticsEnv(): BrowserAnalyticsEnv {
  return {
    enabledFlag: (process.env.NEXT_PUBLIC_POSTHOG_ENABLED ?? "false").trim().toLowerCase(),
    token: process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN ?? "",
    host: process.env.NEXT_PUBLIC_POSTHOG_HOST ?? "https://us.i.posthog.com",
    hasWindow: typeof window !== "undefined",
  };
}

function withPostHog(fn: (client: PostHogClient) => void): void {
  if (posthogClient) {
    fn(posthogClient);
    return;
  }

  if (initPromise) {
    pendingOps.push(fn);
  }
}

function flushPendingOps(): void {
  if (!posthogClient || pendingOps.length === 0) return;
  for (const op of pendingOps.splice(0)) {
    // Isolate each queued op: one throwing PostHog call must not abort the
    // rest of the queue or bubble into the init promise's catch (which would
    // discard the already-initialized client and drop unrelated events). This
    // runs after createAnalytics's wrapper has returned, so it needs its own
    // guard.
    try {
      op(posthogClient);
    } catch (err) {
      console.warn("analytics: queued operation failed", err);
    }
  }
}

function defaultRetryScheduler(retry: () => void, attempt: number): void {
  if (typeof window === "undefined") return;
  // Linear backoff: 500ms, 1000ms, ... bounded by MAX_INIT_ATTEMPTS callers.
  window.setTimeout(retry, 500 * attempt);
}

export function __resetBrowserAnalyticsForTests(): void {
  initialized = false;
  initPromise = null;
  posthogClient = null;
  pendingOps.length = 0;
  initAttempts = 0;
}
