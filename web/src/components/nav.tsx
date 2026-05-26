import Link from "next/link";
import { WalletWidget } from "./wallet-widget";

const CHAIN_NAME_LOWER = (
  process.env.NEXT_PUBLIC_EXPECTED_CHAIN_NAME ?? "Base Sepolia"
).toLowerCase();

export function Nav() {
  return (
    <header className="sticky top-0 z-40 border-b border-ink bg-paper/95 backdrop-blur supports-[backdrop-filter]:bg-paper/85">
      <div className="mx-auto flex max-w-[1280px] items-center justify-between gap-4 px-6 py-3 lg:px-12">
        <Link href="/" className="flex items-baseline gap-3">
          <span className="font-display text-xl leading-none text-ink">MicroAI Paygate</span>
          <span className="hidden font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft sm:inline">
            x402 · {CHAIN_NAME_LOWER}
          </span>
        </Link>
        <div className="flex items-center gap-3">
          <Link
            href="/docs"
            className="border border-ink bg-accent px-3 py-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-paper transition-colors duration-150 hover:bg-accent-press"
          >
            Docs
          </Link>
          <WalletWidget />
          <a
            href="https://github.com/AnkanMisra/MicroAI-Paygate"
            target="_blank"
            rel="noreferrer"
            className="hidden border border-ink bg-paper px-3 py-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-ink transition-colors duration-150 hover:bg-ink hover:text-paper md:inline-flex"
          >
            View source ↗
          </a>
        </div>
      </div>
    </header>
  );
}
