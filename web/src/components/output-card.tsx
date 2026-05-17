import type { SignedReceipt } from "@/lib/verify-receipt";
import { CopyButton } from "./copy-button";

type Props = {
  summary: string;
  receipt: SignedReceipt | null;
};

export function OutputCard({ summary, receipt }: Props) {
  if (!summary) return null;

  return (
    <article className="reveal-up border border-ink bg-paper">
      <header className="flex items-baseline justify-between border-b border-ink px-5 py-3">
        <div className="flex items-baseline gap-3">
          <span className="font-mono text-[10px] uppercase tracking-[0.16em] tnum text-ink-soft">
            Output
          </span>
          {receipt && (
            // Neutral copy — receipt presence proves the gateway sent one, not
            // that it has been cryptographically verified. Green "✓ Valid"
            // lives on the receipt-card after the user clicks Verify signature
            // and the keccak/ECDSA recovery actually runs.
            <span className="font-display text-sm italic text-ink-soft">receipt returned</span>
          )}
        </div>
        <CopyButton value={summary} label="Copy summary" />
      </header>
      <div className="px-5 py-5">
        <p className="font-sans text-[15px] leading-relaxed text-ink whitespace-pre-wrap">
          {summary}
        </p>
      </div>
      {receipt && (
        <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-ink bg-paper-deep px-5 py-3">
          <div className="min-w-0 flex items-center gap-2">
            <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
              Receipt
            </span>
            <code className="truncate font-mono text-xs tnum text-ink">
              {receipt.receipt.id}
            </code>
          </div>
          <CopyButton value={receipt.receipt.id} label="Copy receipt ID" />
        </footer>
      )}
    </article>
  );
}
