"use client";

import { useSyncExternalStore } from "react";
import {
  clearReceipts,
  getReceiptsServerSnapshot,
  getReceiptsSnapshot,
  subscribeReceipts,
} from "@/lib/receipt-storage";
import { Button } from "./ui/button";
import { ReceiptCard } from "./receipt-card";

export function ReceiptHistory() {
  const entries = useSyncExternalStore(
    subscribeReceipts,
    getReceiptsSnapshot,
    getReceiptsServerSnapshot,
  );

  if (entries.length === 0) {
    return (
      <div className="border border-dashed border-ink-faint bg-paper p-10 text-center">
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
    <div className="space-y-3">
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
