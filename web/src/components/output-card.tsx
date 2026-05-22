"use client";

import { useEffect, useState } from "react";
import { verifyReceipt, type SignedReceipt } from "@/lib/verify-receipt";
import { Badge } from "./ui/badge";
import { CopyButton } from "./copy-button";

type Props = {
  summary: string;
  receipt: SignedReceipt | null;
};

type ReceiptVerifyState = "missing" | "verifying" | "valid" | "invalid";

export function OutputCard({ summary, receipt }: Props) {
  const verifyState = useReceiptVerification(receipt);

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
        <CopyButton value={summary} label="Copy summary" />
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
        {receipt && <CopyButton value={receipt.receipt.id} label="Copy receipt ID" />}
      </footer>
    </article>
  );
}

function useReceiptVerification(receipt: SignedReceipt | null): ReceiptVerifyState {
  const [result, setResult] = useState<{
    id: string | null;
    state: Exclude<ReceiptVerifyState, "missing" | "verifying">;
  }>({ id: null, state: "invalid" });

  useEffect(() => {
    let cancelled = false;
    if (!receipt) return undefined;

    void verifyReceipt(receipt)
      .then((ok) => {
        if (!cancelled) {
          setResult({ id: receipt.receipt.id, state: ok ? "valid" : "invalid" });
        }
      })
      .catch(() => {
        if (!cancelled) {
          setResult({ id: receipt.receipt.id, state: "invalid" });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [receipt]);

  if (!receipt) return "missing";
  if (result.id !== receipt.receipt.id) return "verifying";
  return result.state;
}

function ReceiptStatusBadge({ state }: { state: ReceiptVerifyState }) {
  if (state === "valid") return <Badge tone="ok">✓ Receipt verified</Badge>;
  if (state === "invalid") return <Badge tone="alert">✗ Receipt not verified</Badge>;
  if (state === "verifying") return <Badge tone="muted">Verifying receipt…</Badge>;
  return <Badge tone="muted">Receipt not returned</Badge>;
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
