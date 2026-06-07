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

export function shouldEnablePostHog(): boolean {
  return (
    POSTHOG_ENABLED !== "false" &&
    POSTHOG_ENABLED !== "0" &&
    POSTHOG_TOKEN.length > 0 &&
    typeof window !== "undefined"
  );
}
