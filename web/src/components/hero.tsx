import Link from "next/link";
import { ProtocolOrchestra3D } from "./hero-3d/protocol-orchestra";

const DISPLAY_AMOUNT = process.env.NEXT_PUBLIC_PAYMENT_AMOUNT ?? "0.001";
const DISPLAY_TOKEN = process.env.NEXT_PUBLIC_PAYMENT_TOKEN ?? "USDC";
const DISPLAY_CHAIN_NAME =
  process.env.NEXT_PUBLIC_EXPECTED_CHAIN_NAME ?? "Base Sepolia";
const DISPLAY_CHAIN_ID =
  process.env.NEXT_PUBLIC_EXPECTED_CHAIN_ID ?? "84532";

const TICKER_ITEMS = [
  "x402",
  "pay-per-call AI",
  `${DISPLAY_AMOUNT} ${DISPLAY_TOKEN} per request`,
  "EIP-712 typed data",
  "ECDSA receipts",
  `${DISPLAY_CHAIN_NAME} · ${DISPLAY_CHAIN_ID}`,
  "open source · MIT",
  "keccak-256 hashing",
  "verified client-side",
];

const TICKER_RUN = Array.from({ length: 3 }, () => TICKER_ITEMS).flat();

export function Hero() {
  return (
    <section
      className="relative flex flex-col border-b border-ink"
      style={{ minHeight: "calc(100svh - 57px)" }}
    >
      <Ticker />

      <div className="relative mx-auto flex w-full max-w-[1280px] flex-1 flex-col justify-center px-6 py-6 lg:px-12 lg:py-8">
        {/* Editorial paper watermarks — pure depth, never interactive.
            All three are aria-hidden, pointer-events-none, ~4-6% ink so they
            read as ghosted-print under the foreground copy. */}
        <div
          aria-hidden
          className="pointer-events-none absolute -left-2 -top-2 select-none lg:-top-6"
          style={{
            fontFamily: "var(--font-display)",
            fontStyle: "italic",
            fontWeight: 400,
            lineHeight: 0.8,
            fontSize: "clamp(180px, 26vw, 380px)",
            color: "color-mix(in srgb, var(--ink) 5%, transparent)",
          }}
        >
          01
        </div>

        {/* X402 — bottom-right, italic, mirrors the 01 weight */}
        <div
          aria-hidden
          className="pointer-events-none absolute bottom-0 right-0 select-none lg:-bottom-2 lg:-right-4"
          style={{
            fontFamily: "var(--font-display)",
            fontStyle: "italic",
            fontWeight: 400,
            lineHeight: 0.8,
            fontSize: "clamp(120px, 18vw, 260px)",
            color: "color-mix(in srgb, var(--ink) 4%, transparent)",
          }}
        >
          x402
        </div>

        {/* Vertical spine text on the far left — magazine binding cue */}
        <div
          aria-hidden
          className="pointer-events-none absolute left-2 top-1/2 hidden -translate-y-1/2 select-none lg:block"
          style={{
            writingMode: "vertical-rl",
            transform: "translateY(-50%) rotate(180deg)",
            fontFamily: "var(--font-mono)",
            fontSize: "10px",
            letterSpacing: "0.32em",
            textTransform: "uppercase",
            color: "color-mix(in srgb, var(--ink) 32%, transparent)",
          }}
        >
          MicroAI Paygate · Issue 01 · 2026
        </div>

        <div className="reveal-up relative">
          <span className="inline-block font-mono text-[10px] uppercase tracking-[0.22em] text-ink-soft">
            Issue 01 · 2026 · x402 reference implementation
          </span>
        </div>

        <div className="relative mt-4 grid items-center gap-6 lg:mt-6 lg:grid-cols-[1.05fr_0.95fr] lg:gap-10">
          {/* LEFT — copy column */}
          <div className="relative">
            <p
              className="reveal-up mb-3 font-display italic text-ink-soft"
              style={{
                animationDelay: "30ms",
                fontSize: "clamp(15px, 1.4vw, 19px)",
                lineHeight: 1.3,
              }}
            >
              An open-source reference for HTTP{" "}
              <span className="text-accent">402</span> — runnable, end-to-end, right in this browser tab.
            </p>

            <h1
              className="reveal-up font-display tracking-tight leading-[0.92] text-ink"
              style={{
                animationDelay: "120ms",
                fontSize: "clamp(40px, 6vw, 88px)",
              }}
            >
              Pay-per-call AI,
              <br />
              <span className="italic text-accent">authorized</span> on {DISPLAY_CHAIN_NAME}.
            </h1>

            <p
              className="reveal-up mt-5 max-w-xl font-sans text-[15px] leading-relaxed text-ink-soft md:text-base lg:text-[17px]"
              style={{ animationDelay: "240ms" }}
            >
              <span
                className="float-left mr-2 mt-1 font-display italic text-ink"
                style={{
                  fontSize: "clamp(48px, 5vw, 72px)",
                  lineHeight: 0.78,
                }}
              >
                A
              </span>
              n HTTP 402 reference, end-to-end. The wallet signs <em>once</em> with EIP-712.
              The Rust verifier checks the signature. The AI provider runs the request. The gateway
              returns a signed receipt — <strong className="text-ink">verifiable right here</strong>,
              in this browser tab.
            </p>

            <div
              className="reveal-up mt-7 flex flex-wrap items-center gap-3"
              style={{ animationDelay: "360ms" }}
            >
              <a
                href="#try"
                className="group inline-flex items-center justify-center gap-2 border border-ink bg-ink px-5 py-2.5 font-sans text-[12px] font-medium uppercase tracking-[0.08em] text-paper transition-all duration-150 shadow-[6px_6px_0_0_var(--accent)] hover:shadow-[8px_8px_0_0_var(--accent)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-[2px_2px_0_0_var(--accent)]"
              >
                Try it now
                <span aria-hidden className="transition-transform group-hover:translate-y-0.5">↓</span>
              </a>
              <Link
                href="/docs"
                className="inline-flex items-center justify-center gap-2 border border-ink bg-paper px-5 py-2.5 font-sans text-[12px] font-medium uppercase tracking-[0.08em] text-ink transition-all duration-150 shadow-[4px_4px_0_0_var(--ink)] hover:bg-ink hover:text-paper hover:shadow-[6px_6px_0_0_var(--accent)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-[1px_1px_0_0_var(--ink)]"
              >
                Read docs
              </Link>
            </div>
          </div>

          {/* RIGHT — live protocol orchestra */}
          <div
            className="reveal-up relative border border-ink bg-paper"
            style={{ animationDelay: "440ms" }}
          >
            <ProtocolOrchestra3D />
          </div>
        </div>
      </div>

      <div className="border-t border-ink">
        <div className="mx-auto flex max-w-[1280px] divide-x divide-ink">
          <Stat label="Fee per call" value={`${DISPLAY_AMOUNT} ${DISPLAY_TOKEN}`} />
          <Stat label="Chain" value={`${DISPLAY_CHAIN_NAME} · ${DISPLAY_CHAIN_ID}`} />
          <Stat label="Auth" value="EIP-712 typed data" hideOnMobile />
          <Stat label="Receipt" value="ECDSA · keccak-256" hideOnMobile />
        </div>
      </div>
    </section>
  );
}

function Ticker() {
  return (
    <>
      <div className="border-b border-ink bg-ink px-4 py-2 text-center font-mono text-[10px] uppercase tracking-[0.16em] text-paper sm:hidden">
        x402-style · signed receipts · {DISPLAY_CHAIN_NAME}
      </div>
      <div className="relative hidden h-8 overflow-hidden border-b border-ink bg-ink [contain:paint] sm:block">
        <div
          className="ticker-run absolute inset-y-0 left-0 flex items-center whitespace-nowrap font-mono text-[10px] uppercase tracking-[0.22em] text-paper"
          aria-hidden
        >
          {TICKER_RUN.map((item, i) => (
            <span key={`${item}-${i}`} className="flex items-center">
              <span aria-hidden className="px-4 text-accent">
                ✦
              </span>
              <span>{item}</span>
            </span>
          ))}
        </div>
      </div>
    </>
  );
}

function Stat({
  label,
  value,
  hideOnMobile = false,
}: {
  label: string;
  value: string;
  hideOnMobile?: boolean;
}) {
  return (
    <div className={`flex-1 px-5 py-3 lg:px-8 lg:py-3.5 ${hideOnMobile ? "hidden lg:block" : ""}`}>
      <div className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">{label}</div>
      <div className="mt-0.5 font-display text-lg tnum text-ink md:text-xl lg:text-2xl">{value}</div>
    </div>
  );
}
