"use client";

import { useEffect, useRef, useState } from "react";

type Props = {
  value: string;
  label?: string;
  copiedLabel?: string;
  className?: string;
  ariaLabel?: string;
};

export function CopyButton({
  value,
  label = "Copy",
  copiedLabel = "Copied",
  className = "",
  ariaLabel,
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
