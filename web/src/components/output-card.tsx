"use client";

import { useEffect, useState } from "react";
import { AnalyticsEvent } from "@/lib/analytics-events";
import { verifyReceipt, type SignedReceipt } from "@/lib/verify-receipt";
import { Badge } from "./ui/badge";
import { CopyButton } from "./copy-button";

type Props = {
  summary: string;
  receipt: SignedReceipt | null;
  verifyState: ReceiptVerifyState;
};

export type ReceiptVerifyState = "missing" | "verifying" | "valid" | "invalid";

/**
 * Render a card showing the output summary, receipt status, and copy controls.
 *
 * The component returns null if `summary` is falsy. When rendered, it displays:
 * - A header with a receipt verification badge and a copy control for the `summary`.
 * - The `summary` text with preserved whitespace.
 * - A footer that shows the receipt ID (or "not returned") and a descriptive receipt-state message.
 *
 * Copy actions emit analytics: copying the summary triggers `AnalyticsEvent.SummaryCopied`
 * with `{ summary_char_count, has_receipt }`; copying the receipt ID (when present)
 * triggers `AnalyticsEvent.ReceiptIdCopied` with `{ has_receipt: true }`.
 *
 * @param props.summary - The output text to display; if falsy, the component renders `null`.
 * @param props.receipt - The signed receipt object or `null`; when provided the receipt ID is shown and can be copied.
 * @returns An article element containing the summary, receipt status, and copy buttons, or `null` if `summary` is falsy.
 */
export function OutputCard({ summary, receipt, verifyState }: Props) {
  if (!summary) return null;

  return (
    <article className="reveal-up border border-ink bg-paper">
      <header className="flex items-baseline justify-between border-b border-ink px-5 py-3">
        <div className="flex items-baseline gap-3">
          <span className="font-mono text-[10px] uppercase tracking-[0.16em] tnum text-ink-soft">
            Output
          </span>
          <ReceiptStatusBadge state={verifyState} />
        </div>
        <CopyButton
          value={summary}
          label="Copy summary"
          analyticsEvent={AnalyticsEvent.SummaryCopied}
          analyticsProperties={{ summary_char_count: summary.length, has_receipt: !!receipt }}
        />
      </header>
      <div className="px-5 py-5">
        <p className="font-sans text-[15px] leading-relaxed text-ink whitespace-pre-wrap">
          {summary}
        </p>
      </div>
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-ink bg-paper-deep px-5 py-3">
        <div className="min-w-0 space-y-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
              Receipt
            </span>
            {receipt ? (
              <code className="truncate font-mono text-xs tnum text-ink">
                {receipt.receipt.id}
              </code>
            ) : (
              <span className="font-mono text-xs text-ink-soft">not returned</span>
            )}
          </div>
          <p className="font-sans text-xs text-ink-soft">
            {describeReceiptState(verifyState)}
          </p>
        </div>
        {receipt && (
          <CopyButton
            value={receipt.receipt.id}
            label="Copy receipt ID"
            analyticsEvent={AnalyticsEvent.ReceiptIdCopied}
            analyticsProperties={{ has_receipt: true }}
          />
        )}
      </footer>
    </article>
  );
}

export function useReceiptVerification(receipt: SignedReceipt | null): ReceiptVerifyState {
  const receiptKey = getReceiptVerificationKey(receipt);
  const [result, setResult] = useState<{
    key: string | null;
    state: Exclude<ReceiptVerifyState, "missing" | "verifying">;
  }>({ key: null, state: "invalid" });

  useEffect(() => {
    let cancelled = false;
    if (!receipt || !receiptKey) return undefined;

    void verifyReceipt(receipt)
      .then((ok) => {
        if (!cancelled) {
          setResult({ key: receiptKey, state: ok ? "valid" : "invalid" });
        }
      })
      .catch(() => {
        if (!cancelled) {
          setResult({ key: receiptKey, state: "invalid" });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [receipt, receiptKey]);

  if (!receipt) return "missing";
  if (result.key !== receiptKey) return "verifying";
  return result.state;
}

export function getReceiptVerificationKey(receipt: SignedReceipt | null): string | null {
  if (!receipt) return null;

  return JSON.stringify({
    receipt: receipt.receipt,
    signature: receipt.signature,
    server_public_key: receipt.server_public_key,
  });
}

function ReceiptStatusBadge({ state }: { state: ReceiptVerifyState }) {
  let content = <Badge tone="muted">Receipt not returned</Badge>;

  if (state === "valid") {
    content = <Badge tone="ok">✓ Receipt verified</Badge>;
  } else if (state === "invalid") {
    content = <Badge tone="alert">✗ Receipt not verified</Badge>;
  } else if (state === "verifying") {
    content = <Badge tone="muted">Verifying receipt…</Badge>;
  }

  return content;
}

function describeReceiptState(state: ReceiptVerifyState): string {
  if (state === "valid") {
    return "The signed X-402 receipt was decoded and its gateway signature verified in this browser.";
  }
  if (state === "invalid") {
    return "The X-402 receipt was decoded, but the gateway signature did not verify.";
  }
  if (state === "verifying") {
    return "The X-402 receipt was decoded and signature verification is running locally.";
  }
  return "The summary succeeded without an X-402-Receipt header, so there is no receipt ID to verify.";
}
