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

export function ReceiptHistory() {
  const rootRef = useRef<HTMLDivElement | null>(null);
  const trackedView = useRef(false);
  const entries = useSyncExternalStore(
    subscribeReceipts,
    getReceiptsSnapshot,
    getReceiptsServerSnapshot,
  );

  useEffect(() => {
    const node = rootRef.current;
    if (!node || trackedView.current) return;

    const observer = new IntersectionObserver(
      (entriesState) => {
        const entry = entriesState[0];
        if (!entry?.isIntersecting || trackedView.current) return;
        trackedView.current = true;
        browserAnalytics.capture(AnalyticsEvent.ReceiptHistoryViewed, {
          receipt_count: entries.length,
        });
        observer.disconnect();
      },
      { threshold: 0.35 },
    );

    observer.observe(node);
    return () => observer.disconnect();
  }, [entries.length]);

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
