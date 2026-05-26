import type { ReactNode } from "react";
import { Footer } from "@/components/footer";
import { Nav } from "@/components/nav";
import { DocsShell } from "@/components/docs/docs-shell";

export default function DocsLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col bg-paper text-ink">
      <Nav />
      <DocsShell>{children}</DocsShell>
      <Footer />
    </div>
  );
}
