import posthog from "posthog-js";
import { createAnalytics, type AnalyticsClient, type AnalyticsSink } from "./analytics";

const POSTHOG_TOKEN = process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN ?? "";
const POSTHOG_HOST = process.env.NEXT_PUBLIC_POSTHOG_HOST ?? "https://us.i.posthog.com";
const POSTHOG_ENABLED = (process.env.NEXT_PUBLIC_POSTHOG_ENABLED ?? "false").toLowerCase();

let initialized = false;

const sink: AnalyticsSink = {
  capture: (event, properties) => {
    posthog.capture(event, properties);
  },
  identify: (distinctId, properties) => {
    posthog.identify(distinctId, properties);
  },
  reset: () => {
    posthog.reset();
  },
};

export const browserAnalytics: AnalyticsClient = createAnalytics(sink, {
  enabled: shouldEnablePostHog(),
});

/**
 * Initializes the PostHog browser SDK with the configured project token and host using the module's preferred settings.
 *
 * This is a no-op if analytics are disabled or the SDK has already been initialized. When run, it configures PostHog to disable autocapture and session recording, sets pageview/capture behavior, and marks the module as initialized.
 */
export function initBrowserAnalytics(): void {
  if (initialized || !shouldEnablePostHog()) return;

  posthog.init(POSTHOG_TOKEN, {
    api_host: POSTHOG_HOST,
    defaults: "2025-05-24",
    autocapture: false,
    capture_pageview: "history_change",
    capture_pageleave: "if_capture_pageview",
    person_profiles: "identified_only",
    disable_session_recording: true,
  });

  initialized = true;
}

/**
 * Determine whether PostHog analytics should be enabled in the current environment.
 *
 * @returns `true` if the enable flag is not `"false"` or `"0"`, `POSTHOG_TOKEN` is non-empty, and code is running in a browser (`window` is defined); `false` otherwise.
 */
export function shouldEnablePostHog(): boolean {
  return (
    POSTHOG_ENABLED !== "false" &&
    POSTHOG_ENABLED !== "0" &&
    POSTHOG_TOKEN.length > 0 &&
    typeof window !== "undefined"
  );
}
