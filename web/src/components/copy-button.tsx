"use client";

import { useEffect, useRef, useState } from "react";
import { browserAnalytics } from "@/lib/browser-analytics";
import type { AnalyticsProperties } from "@/lib/analytics";
import type { AnalyticsEventName } from "@/lib/analytics-events";

type Props = {
  value: string;
  label?: string;
  copiedLabel?: string;
  className?: string;
  ariaLabel?: string;
  analyticsEvent?: AnalyticsEventName;
  analyticsProperties?: AnalyticsProperties;
};

/**
 * Render a button that copies `value` to the clipboard and shows a temporary "Copied" state.
 *
 * When clicked, the component attempts to write `value` to navigator.clipboard, toggles its visual
 * state to indicate success for 1600ms, and optionally records an analytics event.
 *
 * @param value - The string to copy to the clipboard.
 * @param ariaLabel - Optional override for the button's accessible label; if omitted a label is synthesized from `label` and the start of `value`.
 * @param analyticsEvent - Optional analytics event name to record after a successful copy.
 * @param analyticsProperties - Optional analytics properties sent with `analyticsEvent`.
 * @returns The rendered copy button element.
 */
export function CopyButton({
  value,
  label = "Copy",
  copiedLabel = "Copied",
  className = "",
  ariaLabel,
  analyticsEvent,
  analyticsProperties,
}: Props) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);

  // Clear any pending timer on unmount so setCopied(false) never fires after
  // the component is gone (React warning + potential memory churn).
  useEffect(() => {
    return () => {
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    };
  }, []);

  async function onClick() {
    try {
      await navigator.clipboard.writeText(value);
      if (analyticsEvent) {
        browserAnalytics.capture(analyticsEvent, analyticsProperties);
      }
      setCopied(true);
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setCopied(false), 1600);
    } catch {
      /* clipboard blocked — surface silently */
    }
  }

  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={ariaLabel ?? `${label} ${value.slice(0, 24)}`}
      className={[
        "inline-flex items-center gap-1.5 border border-ink bg-paper px-2 py-1 font-mono text-[10px] uppercase tracking-[0.12em] text-ink transition-colors duration-150 hover:bg-ink hover:text-paper focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
      <span className={copied ? "copied-pop" : undefined}>{copied ? copiedLabel : label}</span>
    </button>
  );
}

function CopyIcon() {
  return (
    <svg width="11" height="11" viewBox="0 0 16 16" fill="none" aria-hidden>
      <rect x="4" y="4" width="9" height="9" stroke="currentColor" strokeWidth="1.5" />
      <path d="M3 11V3h8" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg width="11" height="11" viewBox="0 0 16 16" fill="none" aria-hidden>
      <path d="M3 8.5L6.5 12L13 4.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="square" />
    </svg>
  );
}
