"use client";

import { useState } from "react";
import { browserAnalytics } from "@/lib/browser-analytics";
import { AnalyticsEvent } from "@/lib/analytics-events";
import { useX402 } from "@/hooks/use-x402";
import { Button } from "./ui/button";
import { StatusStrip } from "./status-strip";
import { OutputCard, useReceiptVerification, type ReceiptVerifyState } from "./output-card";
import { ErrorBanner } from "./error-banner";

const SAMPLE_PROMPT =
  "Bitcoin: A Peer-to-Peer Electronic Cash System. A purely peer-to-peer version of electronic cash would allow online payments to be sent directly from one party to another without going through a financial institution. Digital signatures provide part of the solution, but the main benefits are lost if a trusted third party is still required to prevent double-spending. We propose a solution to the double-spending problem using a peer-to-peer network. The network timestamps transactions by hashing them into an ongoing chain of hash-based proof-of-work, forming a record that cannot be changed without redoing the proof-of-work.";

// The actual fee is whatever the gateway embeds in the 402 paymentContext.
// This pre-challenge label is informational only; we read it from env so a
// deployer who sets PAYMENT_AMOUNT to something other than the default doesn't
// mislead users before the wallet opens.
const DISPLAY_AMOUNT = process.env.NEXT_PUBLIC_PAYMENT_AMOUNT ?? "0.001";
const DISPLAY_TOKEN = process.env.NEXT_PUBLIC_PAYMENT_TOKEN ?? "USDC";
const DISPLAY_CHAIN_NAME = process.env.NEXT_PUBLIC_EXPECTED_CHAIN_NAME ?? "Base Sepolia";

/**
 * Renders a two-column "Summarize" form UI that accepts text input, initiates a signed summarize flow, and displays progress, errors, or the resulting summary and receipt.
 *
 * The left column contains a controlled textarea, sample-loading action (which records analytics), word/character counts, cost info, and submit/reset controls. The right column shows the current step, an error banner when present, the summary with its receipt when available, or an idle placeholder otherwise.
 *
 * @returns The React element for the summarize form.
 */
export function SummarizeForm() {
  const [input, setInput] = useState("");
  const { submit, reset, step, summary, receipt, error, isRunning } = useX402();
  const verifyState = useReceiptVerification(receipt);

  const wordCount = input.trim() ? input.trim().split(/\s+/).length : 0;
  const charCount = input.length;
  const canSubmit = wordCount > 0 && !isRunning;

  function handleSubmit() {
    if (!canSubmit) return;
    void submit(input);
  }

  function handleReset() {
    reset();
  }

  return (
    <div className="grid gap-8 lg:grid-cols-[1.05fr_1fr]">
      {/* INPUT COLUMN */}
      <section aria-labelledby="input-heading" className="space-y-3">
        <div className="border border-ink bg-paper">
          <div className="flex items-center justify-between border-b border-ink px-4 py-2">
            <h3
              id="input-heading"
              className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft"
            >
              01 — Your text
            </h3>
            <button
              type="button"
              onClick={() => {
                setInput(SAMPLE_PROMPT);
                browserAnalytics.capture(AnalyticsEvent.SamplePromptLoaded, {
                  input_word_count: SAMPLE_PROMPT.trim().split(/\s+/).length,
                  input_char_count: SAMPLE_PROMPT.length,
                });
              }}
              className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-soft transition-colors hover:text-accent"
            >
              Use sample
            </button>
          </div>
          <textarea
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={`Paste any text. The summary returns after a ${DISPLAY_CHAIN_NAME} signature.`}
            rows={9}
            aria-label="Text to summarize"
            className="block w-full resize-none bg-paper p-4 font-sans text-base leading-relaxed text-ink placeholder:text-ink-faint focus:outline-none"
          />
          <div className="flex items-center justify-between border-t border-ink bg-paper-deep px-4 py-2 font-mono text-[10px] uppercase tracking-[0.12em] tnum text-ink-soft">
            <span>
              {wordCount} word{wordCount === 1 ? "" : "s"} · {charCount} chars
            </span>
            <span className="flex items-center gap-2">
              <span>cost</span>
              <span className="text-ink">{DISPLAY_AMOUNT} {DISPLAY_TOKEN}</span>
            </span>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="max-w-md font-sans text-xs leading-relaxed text-ink-soft">
            Wallet signs the EIP-712 challenge — no on-chain transaction, no gas. Receipt is signed
            by the gateway and verifiable in your browser.
          </p>
          <div className="flex items-center gap-2">
            {(summary || error) && (
              <Button size="sm" variant="ghost" onClick={handleReset}>
                Reset
              </Button>
            )}
            <Button onClick={handleSubmit} disabled={!canSubmit}>
              {isRunning ? "Working…" : "Sign & summarize"}
            </Button>
          </div>
        </div>
      </section>

      {/* RESULT COLUMN */}
      <section aria-labelledby="result-heading" className="space-y-4">
        <h3 id="result-heading" className="sr-only">
          Payment flow and result
        </h3>
        <div role="status" aria-live="polite" className="sr-only">
          {getReceiptAnnouncement(summary, verifyState)}
        </div>
        <StatusStrip step={step} hasError={!!error} />
        {error && <ErrorBanner error={error} onRetry={handleSubmit} onDismiss={handleReset} />}
        {summary && <OutputCard summary={summary} receipt={receipt} verifyState={verifyState} />}
        {!summary && !error && step === "idle" && <PlaceholderCard />}
      </section>
    </div>
  );
}
export function getReceiptAnnouncement(
  summary: string | null,
  state: ReceiptVerifyState,
): string {
  if (!summary) return "";

  if (state === "valid") return "Receipt verified.";
  if (state === "invalid") return "Receipt not verified.";
  if (state === "verifying") return "Verifying receipt…";
  return "Receipt not returned.";
}

function PlaceholderCard() {
  return (
    <div className="border border-dashed border-ink-faint bg-paper p-10">
      <p className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
        Awaiting input
      </p>
      <p className="mt-2 font-display text-2xl leading-tight text-ink">
        Sign once.
        <br />
        Get a signed answer back.
      </p>
      <p className="mt-3 font-sans text-sm text-ink-soft">
        The gateway returns <code className="font-mono text-xs text-ink">402 Payment Required</code>{" "}
        with a payment context. Your wallet signs it, the verifier checks the signature, the AI runs
        the request, and a signed receipt is returned in the response header.
      </p>
    </div>
  );
}
