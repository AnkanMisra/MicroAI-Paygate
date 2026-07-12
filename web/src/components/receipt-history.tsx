"use client";

import { useEffect, useRef, useSyncExternalStore } from "react";
import { browserAnalytics } from "@/lib/browser-analytics";
import { AnalyticsEvent } from "@/lib/analytics-events";
import {
  clearReceipts,
  getReceiptsServerSnapshot,
  getReceiptsSnapshot,
  subscribeReceipts,
} from "@/lib/receipt-storage";
import { Button } from "./ui/button";
import { ReceiptCard } from "./receipt-card";

/**
 * Render the receipts history UI and track first-visibility analytics.
 *
 * Renders an empty-state card when there are no saved receipts, otherwise renders a list of receipt cards with a footer that shows the count and a "Clear local history" action. When the component's root element first becomes visible in the viewport, captures a `ReceiptHistoryViewed` browser analytics event with the current `receipt_count`.
 *
 * @returns The React element for the receipts history UI.
 */
export function ReceiptHistory() {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const trackedView = useRef(false);
  const entries = useSyncExternalStore(
    subscribeReceipts,
    getReceiptsSnapshot,
    getReceiptsServerSnapshot,
  );
  const entriesLengthRef = useRef(entries.length);

  useEffect(() => {
    entriesLengthRef.current = entries.length;
  }, [entries.length]);

  useEffect(() => {
    const node = rootRef.current;
    if (!node || trackedView.current) return;

    const observer = new IntersectionObserver(
      (entriesState) => {
        const entry = entriesState[0];
        if (!entry?.isIntersecting || trackedView.current) return;
        trackedView.current = true;
        browserAnalytics.capture(AnalyticsEvent.ReceiptHistoryViewed, {
          receipt_count: entriesLengthRef.current,
        });
        observer.disconnect();
      },
      { threshold: 0.35 },
    );

    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  if (entries.length === 0) {
    return (
      <div ref={rootRef} className="border border-dashed border-ink-faint bg-paper p-10 text-center">
        <p className="font-mono text-[11px] uppercase tracking-[0.16em] text-ink-soft">
          No receipts yet
        </p>
        <p className="mt-2 font-sans text-sm text-ink-soft">
          Sign a payment above and your receipt will appear here — verifiable client-side.
        </p>
      </div>
    );
  }

  return (
    <div ref={rootRef} className="space-y-3">
      <ul className="space-y-2">
        {entries.map((entry) => (
          <ReceiptCard
            key={entry.receipt.receipt.id}
            signed={entry.receipt}
            savedAt={entry.savedAt}
            promptPreview={entry.promptPreview}
          />
        ))}
      </ul>
      <div className="flex items-center justify-between">
        <p className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-faint">
          {entries.length} receipt{entries.length === 1 ? "" : "s"} · stored in this browser only
        </p>
        <Button size="sm" variant="ghost" onClick={() => clearReceipts()}>
          Clear local history
        </Button>
      </div>
    </div>
  );
}
