import type { ReactNode } from "react";
import Link from "next/link";
import { DocsSidebar } from "./docs-sidebar";

const RESOURCES = [
  {
    label: "Gateway OpenAPI",
    href: "https://github.com/AnkanMisra/MicroAI-Paygate/blob/main/gateway/openapi.yaml",
  },
  {
    label: "SDK README",
    href: "https://github.com/AnkanMisra/MicroAI-Paygate/tree/main/sdk/typescript",
  },
  {
    label: "Deployment Guide",
    href: "https://github.com/AnkanMisra/MicroAI-Paygate/blob/main/DEPLOY.md",
  },
  {
    label: "Security Policy",
    href: "https://github.com/AnkanMisra/MicroAI-Paygate/blob/main/SECURITY.md",
  },
];

export function DocsShell({ children }: { children: ReactNode }) {
  return (
    <main className="flex-1 bg-paper text-ink">
      <section className="border-b border-ink bg-paper-deep">
        <div className="mx-auto max-w-[1280px] px-6 py-10 lg:px-12">
          <div className="flex flex-wrap items-end justify-between gap-6">
            <div>
              <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-ink-soft">
                MicroAI Paygate docs
              </p>
              <div className="mt-2 font-display text-[44px] leading-[0.95] tracking-tight text-ink md:text-[72px]">
                Build against the <span className="italic">paygate.</span>
              </div>
            </div>
            <div className="max-w-md text-sm leading-7 text-ink-soft">
              Developer documentation for the SDK, gateway API, custom x402-style flow,
              operations, and current production limits.
            </div>
          </div>
        </div>
      </section>

      <div className="mx-auto grid w-full max-w-[1280px] min-w-0 grid-cols-1 gap-8 px-6 py-8 lg:grid-cols-[260px_minmax(0,1fr)_220px] lg:px-12 lg:py-12">
        <aside className="min-w-0 lg:sticky lg:top-[82px] lg:self-start">
          <DocsSidebar />
        </aside>

        <article className="min-w-0 border-x-0 border-ink lg:border-x lg:px-8">
          <div className="docs-article mx-auto w-full max-w-3xl pb-16">{children}</div>
        </article>

        <aside className="hidden lg:block lg:sticky lg:top-[82px] lg:self-start">
          <div className="border border-ink bg-paper p-4">
            <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-soft">
              Resources
            </p>
            <div className="mt-4 space-y-2">
              {RESOURCES.map((resource) => (
                <a
                  key={resource.href}
                  href={resource.href}
                  target="_blank"
                  rel="noreferrer"
                  className="block border border-ink bg-paper px-3 py-2 font-mono text-[10px] uppercase tracking-[0.12em] text-ink transition-colors hover:bg-ink hover:text-paper"
                >
                  {resource.label} ↗
                </a>
              ))}
            </div>
          </div>

          <Link
            href="/#protocol"
            className="mt-3 block border border-ink bg-accent px-3 py-2 font-mono text-[10px] uppercase tracking-[0.12em] text-paper transition-colors hover:bg-accent-press"
          >
            View live protocol section
          </Link>
        </aside>
      </div>
    </main>
  );
}
