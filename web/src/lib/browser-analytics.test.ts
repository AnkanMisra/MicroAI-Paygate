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

  it("loads and initializes PostHog only once", async () => {
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
      capture_pageview: false,
      capture_pageleave: "if_capture_pageview",
      person_profiles: "identified_only",
      disable_session_recording: true,
    });
  });

  it("gates the first pageview until after identity reconciliation", async () => {
    const calls: string[] = [];
    const init = mock(() => {
      calls.push("init");
    });
    const capture = mock((event: string) => {
      calls.push(`capture:${event}`);
    });
    const setConfig = mock(() => {
      calls.push("set_config");
    });
    const loadPostHog = mock(async () => ({
      default: {
        init,
        capture,
        set_config: setConfig,
        identify: mock(() => undefined),
        reset: mock(() => undefined),
      },
    })) as unknown as Parameters<typeof initBrowserAnalytics>[0];

    const resolveLiveWallet = mock(async () => {
      calls.push("resolve");
      return null;
    });
    const schedule = (() => undefined) as Parameters<typeof initBrowserAnalytics>[1];

    initBrowserAnalytics(loadPostHog, schedule, resolveLiveWallet);
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();

    // init must request capture_pageview: false so PostHog does not auto-fire a
    // pageview before reconciliation.
    expect(init).toHaveBeenCalledWith(
      "phc_test",
      expect.objectContaining({ capture_pageview: false }),
    );
    // The live wallet must be resolved BEFORE the manual pageview is captured,
    // and capture_pageview is only enabled afterwards.
    expect(resolveLiveWallet).toHaveBeenCalledTimes(1);
    expect(calls.indexOf("resolve")).toBeLessThan(calls.indexOf("capture:$pageview"));
    expect(calls.indexOf("capture:$pageview")).toBeLessThan(calls.indexOf("set_config"));
    expect(setConfig).toHaveBeenCalledWith({ capture_pageview: "history_change" });
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

  it("auto-retries initialization via the scheduler after a transient load failure", async () => {
    let call = 0;
    const init = mock(() => undefined);
    const load = mock(async () => {
      call += 1;
      if (call === 1) throw new Error("network down");
      return {
        default: {
          init,
          capture: mock(() => undefined),
          identify: mock(() => undefined),
          reset: mock(() => undefined),
        },
      };
    }) as unknown as Parameters<typeof initBrowserAnalytics>[0];

    const scheduled: Array<() => void> = [];
    const schedule = (retry: () => void) => {
      scheduled.push(retry);
    };

    initBrowserAnalytics(load, schedule);
    await Promise.resolve();
    await Promise.resolve();

    // First load failed and a retry was scheduled — but NOT yet run. The single
    // production caller never calls init twice, so the scheduler is what makes
    // recovery happen without a full page reload.
    expect(load).toHaveBeenCalledTimes(1);
    expect(init).not.toHaveBeenCalled();
    expect(scheduled).toHaveLength(1);

    scheduled[0]();
    await Promise.resolve();
    await Promise.resolve();

    expect(load).toHaveBeenCalledTimes(2);
    expect(init).toHaveBeenCalledTimes(1);
  });

  it("stops retrying after the bounded number of attempts", async () => {
    const load = mock(async () => {
      throw new Error("permanently down");
    }) as unknown as Parameters<typeof initBrowserAnalytics>[0];

    const scheduled: Array<() => void> = [];
    const schedule = (retry: () => void) => {
      scheduled.push(retry);
    };

    initBrowserAnalytics(load, schedule);
    await Promise.resolve();
    await Promise.resolve();

    let guard = 0;
    while (scheduled.length > 0 && guard < 10) {
      const next = scheduled.shift()!;
      next();
      await Promise.resolve();
      await Promise.resolve();
      guard += 1;
    }

    // MAX_INIT_ATTEMPTS = 3 → three load attempts total, then it gives up.
    expect(load).toHaveBeenCalledTimes(3);
  });
});
