import { afterEach, beforeEach, describe, expect, it, mock } from "bun:test";
import {
  __resetBrowserAnalyticsForTests,
  initBrowserAnalytics,
  shouldEnablePostHog,
} from "./browser-analytics";

describe("shouldEnablePostHog", () => {
  it("requires an enabled flag, token, and browser window", () => {
    expect(
      shouldEnablePostHog({
        enabledFlag: "true",
        token: "phc_test",
        host: "https://us.i.posthog.com",
        hasWindow: true,
      }),
    ).toBe(true);

    expect(
      shouldEnablePostHog({
        enabledFlag: "false",
        token: "phc_test",
        host: "https://us.i.posthog.com",
        hasWindow: true,
      }),
    ).toBe(false);

    expect(
      shouldEnablePostHog({
        enabledFlag: "1",
        token: "",
        host: "https://us.i.posthog.com",
        hasWindow: true,
      }),
    ).toBe(false);

    expect(
      shouldEnablePostHog({
        enabledFlag: "1",
        token: "phc_test",
        host: "https://us.i.posthog.com",
        hasWindow: false,
      }),
    ).toBe(false);

    expect(
      shouldEnablePostHog({
        enabledFlag: "TRUE",
        token: "phc_test",
        host: "https://us.i.posthog.com",
        hasWindow: true,
      }),
    ).toBe(false);

    expect(
      shouldEnablePostHog({
        enabledFlag: "yes",
        token: "phc_test",
        host: "https://us.i.posthog.com",
        hasWindow: true,
      }),
    ).toBe(false);
  });
});

describe("initBrowserAnalytics", () => {
  const originalEnabled = process.env.NEXT_PUBLIC_POSTHOG_ENABLED;
  const originalToken = process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN;
  const originalHost = process.env.NEXT_PUBLIC_POSTHOG_HOST;
  const originalWindow = globalThis.window;

  beforeEach(() => {
    __resetBrowserAnalyticsForTests();
    process.env.NEXT_PUBLIC_POSTHOG_ENABLED = "true";
    process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN = "phc_test";
    process.env.NEXT_PUBLIC_POSTHOG_HOST = "https://us.i.posthog.com";
    (globalThis as { window?: typeof globalThis.window }).window = {} as typeof globalThis.window;
  });

  afterEach(() => {
    __resetBrowserAnalyticsForTests();

    if (originalEnabled === undefined) {
      delete process.env.NEXT_PUBLIC_POSTHOG_ENABLED;
    } else {
      process.env.NEXT_PUBLIC_POSTHOG_ENABLED = originalEnabled;
    }

    if (originalToken === undefined) {
      delete process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN;
    } else {
      process.env.NEXT_PUBLIC_POSTHOG_PROJECT_TOKEN = originalToken;
    }

    if (originalHost === undefined) {
      delete process.env.NEXT_PUBLIC_POSTHOG_HOST;
    } else {
      process.env.NEXT_PUBLIC_POSTHOG_HOST = originalHost;
    }

    if (originalWindow === undefined) {
      delete (globalThis as { window?: typeof globalThis.window }).window;
    } else {
      (globalThis as { window?: typeof globalThis.window }).window = originalWindow;
    }
  });

  it("loads and initializes PostHog only once with the expected config", async () => {
    const init = mock(() => undefined);
    const loadPostHog = mock(async () => ({
      default: {
        init,
        capture: mock(() => undefined),
        identify: mock(() => undefined),
        reset: mock(() => undefined),
      },
    })) as unknown as Parameters<typeof initBrowserAnalytics>[0];

    initBrowserAnalytics(loadPostHog);
    initBrowserAnalytics(loadPostHog);
    await Promise.resolve();
    await Promise.resolve();

    expect(loadPostHog).toHaveBeenCalledTimes(1);
    expect(init).toHaveBeenCalledTimes(1);
    expect(init).toHaveBeenCalledWith("phc_test", {
      api_host: "https://us.i.posthog.com",
      defaults: "2025-05-24",
      autocapture: false,
      capture_pageview: "history_change",
      capture_pageleave: "if_capture_pageview",
      person_profiles: "identified_only",
      disable_session_recording: true,
    });
  });

  it("skips loading when PostHog is disabled", () => {
    process.env.NEXT_PUBLIC_POSTHOG_ENABLED = "false";
    const loadPostHog = mock(async () => {
      throw new Error("should not load");
    }) as unknown as Parameters<typeof initBrowserAnalytics>[0];

    initBrowserAnalytics(loadPostHog);

    expect(loadPostHog).not.toHaveBeenCalled();
  });

  it("accepts explicitly enabled values after env normalization", async () => {
    process.env.NEXT_PUBLIC_POSTHOG_ENABLED = " TRUE ";
    const init = mock(() => undefined);
    const loadPostHog = mock(async () => ({
      default: {
        init,
        capture: mock(() => undefined),
        identify: mock(() => undefined),
        reset: mock(() => undefined),
      },
    })) as unknown as Parameters<typeof initBrowserAnalytics>[0];

    initBrowserAnalytics(loadPostHog);
    await Promise.resolve();
    await Promise.resolve();

    expect(loadPostHog).toHaveBeenCalledTimes(1);
    expect(init).toHaveBeenCalledTimes(1);
  });
});
