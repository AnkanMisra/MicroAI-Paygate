import "../test/setup-dom";

import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, mock } from "bun:test";
import { CopyButton } from "./copy-button";
import { browserAnalytics } from "@/lib/browser-analytics";
import { AnalyticsEvent } from "@/lib/analytics-events";

const originalCapture = browserAnalytics.capture;
const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalSetTimeout = window.setTimeout;
const originalClearTimeout = window.clearTimeout;

let nextTimerId = 1;
let pendingTimers = new Map<number, () => void>();

function installDeterministicTimers(): void {
  nextTimerId = 1;
  pendingTimers = new Map();
  window.setTimeout = ((callback: TimerHandler) => {
    const id = nextTimerId++;
    pendingTimers.set(id, callback as () => void);
    return id;
  }) as unknown as typeof window.setTimeout;
  window.clearTimeout = ((id: number) => {
    pendingTimers.delete(id);
  }) as typeof window.clearTimeout;
}

function flushPendingTimer(): void {
  const [id, callback] = pendingTimers.entries().next().value ?? [];
  if (id !== undefined && callback) {
    pendingTimers.delete(id);
    callback();
  }
}

function installClipboard(writeText = mock(async (_value: string) => undefined)) {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  return writeText;
}

beforeEach(() => {
  installDeterministicTimers();
  installClipboard();
  browserAnalytics.capture = mock();
});

afterEach(() => {
  cleanup();
  window.setTimeout = originalSetTimeout;
  window.clearTimeout = originalClearTimeout;
  browserAnalytics.capture = originalCapture;
  pendingTimers.clear();

  if (originalClipboard) {
    Object.defineProperty(navigator, "clipboard", originalClipboard);
  } else {
    delete (navigator as unknown as { clipboard?: unknown }).clipboard;
  }
});

describe("CopyButton", () => {
  it("synthesizes a bounded accessible label and preserves custom props", () => {
    const longValue = "abcdefghijklmnopqrstuvwxyz0123456789";
    const { getByRole } = render(
      <CopyButton value={longValue} label="Copy code" className="custom-class" />,
    );
    const button = getByRole("button");

    expect(button.getAttribute("aria-label")).toBe("Copy code abcdefghijklmnopqrstuvwx");
    expect(button.className).toContain("custom-class");

    cleanup();
    const custom = render(
      <CopyButton value="secret" label="Copy" ariaLabel="Copy secret value" />,
    ).getByRole("button");
    expect(custom.getAttribute("aria-label")).toBe("Copy secret value");
  });

  it("copies the value, shows the configured label, and emits analytics once", async () => {
    const writeText = installClipboard();
    const { getByRole } = render(
      <CopyButton
        value="hello"
        label="Copy"
        copiedLabel="Copied!"
        analyticsEvent={AnalyticsEvent.SummaryCopied}
        analyticsProperties={{ source: "test" }}
      />,
    );
    const button = getByRole("button");

    fireEvent.click(button);
    await waitFor(() => expect(button.textContent).toContain("Copied!"));

    expect(writeText).toHaveBeenCalledWith("hello");
    expect(browserAnalytics.capture).toHaveBeenCalledTimes(1);
    expect(browserAnalytics.capture).toHaveBeenCalledWith(AnalyticsEvent.SummaryCopied, {
      source: "test",
    });
  });

  it("returns to the normal label after the scheduled reset", async () => {
    const { getByRole } = render(<CopyButton value="hello" />);
    const button = getByRole("button");

    fireEvent.click(button);
    await waitFor(() => expect(button.textContent).toContain("Copied"));
    expect(pendingTimers.size).toBe(1);

    act(() => flushPendingTimer());
    expect(button.textContent).toContain("Copy");
  });

  it("replaces the pending reset on repeated successful clicks", async () => {
    const { getByRole } = render(<CopyButton value="hello" />);
    const button = getByRole("button");

    fireEvent.click(button);
    await waitFor(() => expect(button.textContent).toContain("Copied"));
    const firstTimer = [...pendingTimers.keys()][0];

    fireEvent.click(button);
    await waitFor(() => expect(button.textContent).toContain("Copied"));

    expect(pendingTimers.size).toBe(1);
    expect(pendingTimers.has(firstTimer)).toBe(false);
  });

  it("keeps the normal label and skips analytics when copying fails", async () => {
    const writeText = installClipboard(mock(async () => Promise.reject(new Error("blocked"))));
    const { getByRole } = render(
      <CopyButton value="hello" analyticsEvent={AnalyticsEvent.SummaryCopied} />,
    );
    const button = getByRole("button");

    fireEvent.click(button);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith("hello"));

    expect(button.textContent).toContain("Copy");
    expect(browserAnalytics.capture).not.toHaveBeenCalled();
    expect(pendingTimers.size).toBe(0);
  });

  it("clears a pending reset when unmounted", async () => {
    const { getByRole, unmount } = render(<CopyButton value="hello" />);
    const button = getByRole("button");

    fireEvent.click(button);
    await waitFor(() => expect(button.textContent).toContain("Copied"));
    expect(pendingTimers.size).toBe(1);

    unmount();
    expect(pendingTimers.size).toBe(0);
  });
});
