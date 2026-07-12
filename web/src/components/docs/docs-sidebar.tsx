"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { DOCS_NAV_ITEMS } from "./docs-nav";

export function DocsSidebar() {
  const pathname = usePathname();

  return (
    <nav aria-label="Documentation" className="min-w-0 space-y-4">
      <div>
        <p className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-soft">
          Documentation
        </p>
        <p className="mt-2 text-sm leading-6 text-ink-soft">
          Full-platform reference for building, running, and integrating MicroAI Paygate.
        </p>
      </div>

      <div className="-mx-1 flex w-full max-w-full gap-2 overflow-x-auto pb-2 lg:mx-0 lg:block lg:space-y-1 lg:overflow-visible lg:pb-0">
        {DOCS_NAV_ITEMS.map((item) => {
          const active = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              aria-current={active ? "page" : undefined}
              className={[
                "min-w-[180px] border border-ink px-3 py-2 transition-colors lg:block lg:min-w-0",
                active ? "bg-ink text-paper" : "bg-paper text-ink hover:bg-paper-deep",
              ]
                .filter(Boolean)
                .join(" ")}
            >
              <span className="block font-mono text-[10px] uppercase tracking-[0.14em]">
                {item.label}
              </span>
              <span
                className={[
                  "mt-1 hidden text-xs leading-5 lg:block",
                  active ? "text-paper/75" : "text-ink-soft",
                ]
                  .filter(Boolean)
                  .join(" ")}
              >
                {item.description}
              </span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
