const DISPLAY_CHAIN_NAME =
  process.env.NEXT_PUBLIC_EXPECTED_CHAIN_NAME ?? "Base Sepolia";

const STEPS = [
  {
    num: "01",
    title: "Request",
    desc:
      "Client posts unsigned text to /api/ai/summarize. The gateway never charges before it has a signature.",
  },
  {
    num: "02",
    title: "Challenge",
    desc:
      "Gateway responds 402 with a payment context: recipient, amount, token, nonce, chainId, and timestamp.",
  },
  {
    num: "03",
    title: "Sign",
    desc:
      "Your wallet signs the EIP-712 typed data. No on-chain transaction, no gas — only an authorization signature.",
  },
  {
    num: "04",
    title: "Verify",
    desc:
      "The Rust verifier recovers the signer, enforces chain ID parity, timestamp window, and rejects replayed nonces.",
  },
  {
    num: "05",
    title: "Generate",
    desc:
      "Only after the signature is verified does the gateway call the AI provider. Failure here doesn't charge you.",
  },
  {
    num: "06",
    title: "Receipt",
    desc:
      "Gateway signs a receipt over the request and response hashes. Returned in X-402-Receipt and verifiable client-side.",
  },
];

export function HowItWorks() {
  return (
    <section id="protocol" className="border-t-2 border-ink bg-paper-deep">
      <div className="mx-auto max-w-[1280px] px-6 py-20 lg:px-12 lg:py-28">
        <div className="flex flex-wrap items-end justify-between gap-6 border-b border-ink pb-6">
          <div>
            <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-ink-soft">
              § 03 — The protocol
            </span>
            <h2
              className="mt-2 font-display leading-[0.95] tracking-tight text-ink"
              style={{ fontSize: "clamp(40px, 6vw, 80px)" }}
            >
              The x402 dance,
              <br />
              <span className="italic">in six moves.</span>
            </h2>
          </div>
          <p className="max-w-sm font-sans text-sm leading-relaxed text-ink-soft md:text-base">
            Each step has its own failure mode and a structured error code. The status strip in the
            tool shows where you are in the flow, live.
          </p>
        </div>

        <ol className="mt-10 grid gap-px bg-ink md:grid-cols-2 lg:grid-cols-3">
          {STEPS.map((s) => (
            <li key={s.num} className="group relative bg-paper p-6 transition-colors duration-200 hover:bg-paper-deep lg:p-8">
              <div className="flex items-baseline gap-3">
                <span className="font-mono text-[10px] uppercase tracking-[0.16em] tnum text-ink-soft">
                  {s.num}
                </span>
                <span className="block h-px flex-1 bg-ink-faint" aria-hidden />
              </div>
              <h3 className="mt-3 font-display leading-none text-ink" style={{ fontSize: "clamp(28px, 3.5vw, 44px)" }}>
                {s.title}
              </h3>
              <p className="mt-3 font-sans text-sm leading-relaxed text-ink-soft">
                {s.desc}
              </p>
            </li>
          ))}
        </ol>

        <p className="mt-10 max-w-2xl font-sans text-sm leading-relaxed text-ink-soft">
          A valid signature proves wallet authorization — it does not prove USDC moved on-chain. This
          implementation is a demo on {DISPLAY_CHAIN_NAME}. The cryptography is real; the settlement
          is not.
        </p>
      </div>
    </section>
  );
}
