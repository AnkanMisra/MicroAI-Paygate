import { createAnalytics, type AnalyticsClient, type AnalyticsSink } from "./analytics";

type PostHogClient = (typeof import("posthog-js"))["default"];
type PostHogLoader = () => Promise<{ default: PostHogClient }>;
type BrowserAnalyticsEnv = {
  enabledFlag: string;
  token: string;
  host: string;
  hasWindow: boolean;
};

let initialized = false;
let initPromise: Promise<void> | null = null;
let posthogClient: PostHogClient | null = null;
const pendingOps: Array<(client: PostHogClient) => void> = [];

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
});

/**
 * Initializes the PostHog browser SDK with the configured project token and host using the module's preferred settings.
 *
 * This is a no-op if analytics are disabled or the SDK has already been initialized. When run, it configures PostHog to disable autocapture and session recording, sets pageview/capture behavior, and marks the module as initialized.
 */
export function initBrowserAnalytics(
  loadPostHog: PostHogLoader = () => import("posthog-js"),
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
      flushPendingOps();
    })
    .catch((err) => {
      console.warn("analytics: failed to initialize PostHog", err);
      pendingOps.length = 0;
    });
}

/**
 * Determine whether PostHog analytics should be enabled in the current environment.
 *
 * @returns `true` if the enable flag is not `"false"` or `"0"`, `POSTHOG_TOKEN` is non-empty, and code is running in a browser (`window` is defined); `false` otherwise.
 */
export function shouldEnablePostHog(env: BrowserAnalyticsEnv): boolean {
  return (
    env.enabledFlag !== "false" &&
    env.enabledFlag !== "0" &&
    env.token.length > 0 &&
    env.hasWindow
  );
}

function readBrowserAnalyticsEnv(): BrowserAnalyticsEnv {
  return {
    enabledFlag: (process.env.NEXT_PUBLIC_POSTHOG_ENABLED ?? "false").toLowerCase(),
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
    op(posthogClient);
  }
}

export function __resetBrowserAnalyticsForTests(): void {
  initialized = false;
  initPromise = null;
  posthogClient = null;
  pendingOps.length = 0;
}
