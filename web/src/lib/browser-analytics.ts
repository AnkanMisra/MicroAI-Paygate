import {
  createAnalytics,
  type AnalyticsClient,
  type AnalyticsSink,
  type IdentityStore,
} from "./analytics";

type PostHogClient = (typeof import("posthog-js"))["default"];
type PostHogLoader = () => Promise<{ default: PostHogClient }>;
type RetryScheduler = (retry: () => void, attempt: number) => void;
// Resolves the wallet currently connected in the live provider (or null when no
// provider / no account). Injected so the analytics layer stays decoupled from
// the wallet implementation and remains testable.
type LiveWalletResolver = () => Promise<string | null>;
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
// Bounds the buffer of events queued while the SDK loads / retries, so a long
// outage can't grow it without limit. Oldest events are dropped first.
const MAX_PENDING_OPS = 100;

let initialized = false;
let initPromise: Promise<void> | null = null;
let posthogClient: PostHogClient | null = null;
let initAttempts = 0;
// True once we've exhausted retries: stop queuing so we don't buffer forever.
let gaveUp = false;
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

const defaultResolveLiveWallet: LiveWalletResolver = async () => {
  try {
    const { getCurrentAccount } = await import("@/lib/wallet");
    return await getCurrentAccount();
  } catch {
    return null;
  }
};

// A PostHog distinct_id that looks like an EVM wallet address means a previous
// session identified a wallet. Anonymous ids are UUIDs, so this never matches
// them. Used to reconcile against PostHog's own persisted identity, not just
// our localStorage marker.
const WALLET_ADDRESS_RE = /^0x[0-9a-fA-F]{40}$/;

/**
 * Initializes the PostHog browser SDK, resolving and reconciling wallet identity BEFORE any event is forwarded.
 *
 * No-op if analytics are disabled or already initialized. On the first successful load it:
 * 1. resolves the live wallet first, so there is no async window where events could forward under a stale id;
 * 2. initializes PostHog with autocapture, session recording, and automatic pageview capture all disabled;
 * 3. reconciles identity — resetting if our persisted marker OR PostHog's own persisted `distinct_id`
 *    (when it is wallet-shaped) no longer matches the live wallet — before exposing the client;
 * 4. exposes the client, flushes events queued while loading, and captures the initial `$pageview` manually.
 *
 * Pageview capture is kept manual (init `capture_pageview: false` + an explicit `$pageview`) rather than
 * toggled on via `set_config`, because in the pinned posthog-js the history-change monitor only starts during
 * `init`, so a later `set_config` would not begin SPA pageview capture. If the dynamic import or `posthog.init`
 * fails transiently, a bounded auto-retry is scheduled (up to `MAX_INIT_ATTEMPTS`); queued events are preserved
 * across retries and only dropped once retries are exhausted.
 *
 * @param loadPostHog - Loader for the PostHog module; injectable for tests.
 * @param schedule - Scheduler used to defer a retry after a failed init; injectable for tests. Defaults to a backoff-based `setTimeout`.
 * @param resolveLiveWallet - Resolves the currently connected wallet (or null); injectable for tests.
 */
export function initBrowserAnalytics(
  loadPostHog: PostHogLoader = () => import("posthog-js"),
  schedule: RetryScheduler = defaultRetryScheduler,
  resolveLiveWallet: LiveWalletResolver = defaultResolveLiveWallet,
): void {
  const env = readBrowserAnalyticsEnv();
  if (initialized || initPromise || gaveUp || !shouldEnablePostHog(env)) return;

  initPromise = loadPostHog()
    .then(async ({ default: posthog }) => {
      // Resolve the live wallet BEFORE exposing the client, so no capture/
      // identify can be forwarded during reconciliation under a stale identity.
      const live = await resolveLiveWallet();

      posthog.init(env.token, {
        api_host: env.host,
        defaults: "2025-05-24",
        autocapture: false,
        // Manual pageview only — see the doc comment for why set_config can't
        // re-enable history capture in the pinned SDK.
        capture_pageview: false,
        capture_pageleave: "if_capture_pageview",
        person_profiles: "identified_only",
        disable_session_recording: true,
      });

      // Reconcile against PostHog's OWN persisted identity (not just our marker):
      // if it restored a wallet-shaped distinct_id that no longer matches the
      // live wallet, reset before any event is captured.
      const persistedDistinctId = safeGetDistinctId(posthog);
      const liveLower = live ? live.toLowerCase() : null;
      if (
        persistedDistinctId &&
        WALLET_ADDRESS_RE.test(persistedDistinctId) &&
        persistedDistinctId.toLowerCase() !== liveLower
      ) {
        try {
          posthog.reset();
        } catch (err) {
          console.warn("analytics: failed to reset stale PostHog identity", err);
        }
        identityStore.clear();
      }

      // Also reconcile our own marker (covers the case where PostHog's id is
      // anonymous but our marker is stale).
      browserAnalytics.reconcileIdentity(live);

      // Expose the client only now — after identity is reconciled.
      posthogClient = posthog;
      initialized = true;
      initAttempts = 0;

      flushPendingOps();

      // Fire the initial pageview manually, now that identity is correct.
      try {
        posthog.capture("$pageview");
      } catch (err) {
        console.warn("analytics: failed to capture initial pageview", err);
      }
    })
    .catch((err) => {
      console.warn("analytics: failed to initialize PostHog", err);
      initialized = false;
      initPromise = null;
      posthogClient = null;
      // Keep pendingOps: events queued during this attempt (and the retry
      // delay) must survive so a later successful init can flush them.
      initAttempts += 1;
      if (initAttempts < MAX_INIT_ATTEMPTS) {
        schedule(() => initBrowserAnalytics(loadPostHog, schedule, resolveLiveWallet), initAttempts);
      } else {
        gaveUp = true;
        pendingOps.length = 0;
      }
    });
}

function safeGetDistinctId(posthog: PostHogClient): string | null {
  try {
    return posthog.get_distinct_id();
  } catch {
    return null;
  }
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

  // Not loaded yet — during the initial load OR between retries (when
  // initPromise is briefly null). Queue so events emitted in that window
  // survive until a successful init flushes them, unless we've permanently
  // given up. Bound the buffer so a long outage can't grow it without limit;
  // drop oldest first.
  if (gaveUp) return;
  pendingOps.push(fn);
  if (pendingOps.length > MAX_PENDING_OPS) {
    pendingOps.shift();
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
  gaveUp = false;
}
