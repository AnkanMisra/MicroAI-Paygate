export function Footer() {
  return (
    <footer className="border-t-2 border-ink bg-paper">
      <div className="mx-auto flex max-w-[1280px] flex-wrap items-center justify-between gap-4 px-6 py-6 lg:px-12">
        <div>
          <p className="font-display text-lg leading-none text-ink">MicroAI Paygate</p>
          <p className="mt-1 font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
            MIT · 2026 · open source x402 reference
          </p>
        </div>
        <nav className="flex flex-wrap items-center gap-x-6 gap-y-2 font-mono text-[11px] uppercase tracking-[0.14em] text-ink-soft">
          <a
            href="https://github.com/AnkanMisra/MicroAI-Paygate/blob/main/README.md"
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-ink"
          >
            README
          </a>
          <a
            href="https://github.com/AnkanMisra/MicroAI-Paygate/blob/main/DEPLOY.md"
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-ink"
          >
            Deploy guide
          </a>
          <a
            href="https://github.com/AnkanMisra/MicroAI-Paygate/blob/main/SECURITY.md"
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-ink"
          >
            Security
          </a>
          <a
            href="https://github.com/AnkanMisra/MicroAI-Paygate"
            target="_blank"
            rel="noreferrer"
            className="transition-colors hover:text-ink"
          >
            GitHub ↗
          </a>
        </nav>
      </div>
    </footer>
  );
}
