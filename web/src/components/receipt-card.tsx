"use client";

import { useState } from "react";
import { verifyReceipt, type SignedReceipt } from "@/lib/verify-receipt";
import { getChainMeta, shortenAddress } from "@/lib/wallet";
import { Badge } from "./ui/badge";
import { CopyButton } from "./copy-button";

type Props = {
  signed: SignedReceipt;
  savedAt: number;
  promptPreview: string;
};

type VerifyState = "idle" | "verifying" | "valid" | "invalid";

export function ReceiptCard({ signed, savedAt, promptPreview }: Props) {
  const [expanded, setExpanded] = useState(false);
  const [verifyState, setVerifyState] = useState<VerifyState>("idle");

  const r = signed.receipt;
  const meta = getChainMeta(r.payment.chainId);

  async function handleVerify() {
    setVerifyState("verifying");
    const ok = await verifyReceipt(signed);
    setVerifyState(ok ? "valid" : "invalid");
  }

  return (
    <li className="border border-ink bg-paper transition-colors duration-150 hover:bg-paper-deep">
      <div className="flex flex-wrap items-start justify-between gap-3 p-4">
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <code className="font-mono text-xs text-ink tnum">{r.id}</code>
            <Badge tone="muted">{meta.name}</Badge>
            <time
              dateTime={new Date(savedAt).toISOString()}
              className="font-mono text-[10px] uppercase tracking-[0.12em] tnum text-ink-soft"
            >
              {formatRelative(savedAt)}
            </time>
          </div>
          {promptPreview && (
            <p className="truncate font-sans text-xs italic text-ink-soft">
              “{promptPreview}{promptPreview.length === 80 ? "…" : ""}”
            </p>
          )}
          <div className="font-mono text-[11px] tnum text-ink-soft">
            {shortenAddress(r.payment.payer)}{" "}
            <span aria-hidden className="text-ink-faint">→</span>{" "}
            {shortenAddress(r.payment.recipient)}{" "}
            <span aria-hidden className="text-ink-faint">·</span>{" "}
            <span className="text-ink">{r.payment.amount} {r.payment.token}</span>
          </div>
        </div>
        <div className="flex flex-shrink-0 items-center gap-1.5">
          <CopyButton value={r.id} label="Copy" />
          <VerifyControl state={verifyState} onClick={handleVerify} />
        </div>
      </div>
      <div className="flex items-center justify-between border-t border-ink-faint px-4 py-2">
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft hover:text-accent"
        >
          {expanded ? "− Hide signature & JSON" : "+ Show signature & JSON"}
        </button>
        <code className="hidden truncate font-mono text-[10px] text-ink-faint tnum sm:inline">
          sig {shortenAddress(signed.signature, 6)}
        </code>
      </div>
      {expanded && (
        <pre className="m-4 mt-0 max-h-48 overflow-auto border border-ink-faint bg-paper-deep p-3 font-mono text-[10px] leading-relaxed text-ink-soft">
          {JSON.stringify(signed, null, 2)}
        </pre>
      )}
    </li>
  );
}

function VerifyControl({ state, onClick }: { state: VerifyState; onClick: () => void }) {
  if (state === "verifying") return <Badge tone="muted">Verifying…</Badge>;
  if (state === "valid") return <Badge tone="ok">✓ Signature valid</Badge>;
  if (state === "invalid") return <Badge tone="alert">✗ Invalid</Badge>;
  return (
    <button
      type="button"
      onClick={onClick}
      className="border border-ink bg-paper px-2 py-1 font-mono text-[10px] uppercase tracking-[0.12em] text-ink transition-colors duration-150 hover:bg-ink hover:text-paper focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
    >
      Verify signature
    </button>
  );
}

function formatRelative(ms: number): string {
  const diff = Date.now() - ms;
  if (diff < 60_000) return "just now";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  return `${Math.floor(diff / 86_400_000)}d ago`;
}
