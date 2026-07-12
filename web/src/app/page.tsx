import { Nav } from "@/components/nav";
import { Hero } from "@/components/hero";
import { SummarizeForm } from "@/components/summarize-form";
import { ReceiptHistory } from "@/components/receipt-history";
import { HowItWorks } from "@/components/how-it-works";
import { Footer } from "@/components/footer";

export default function Home() {
  return (
    <div className="flex min-h-screen flex-col bg-paper text-ink">
      <Nav />
      <main className="flex-1">
        <Hero />

        <section id="try" className="border-b border-ink">
          <div className="mx-auto max-w-[1280px] px-6 py-16 lg:px-12 lg:py-24">
            <SectionHeader
              marker="§ 01 — The tool"
              titleLeft="Sign once."
              titleRight="Pay once."
              caption="Paste any text. The gateway will challenge you with a 402, your wallet signs it once, and a summary comes back with a verifiable receipt."
            />
            <SummarizeForm />
          </div>
        </section>

        <section id="receipts" className="border-b border-ink bg-paper-deep">
          <div className="mx-auto max-w-[1280px] px-6 py-16 lg:px-12 lg:py-24">
            <SectionHeader
              marker="§ 02 — Your receipts"
              titleLeft="Each signature,"
              titleRight="filed."
              caption="Every successful payment leaves a signed receipt. Click verify on any of them — the check runs locally in your browser, against the gateway's public key."
            />
            <ReceiptHistory />
          </div>
        </section>

        <HowItWorks />
      </main>
      <Footer />
    </div>
  );
}

function SectionHeader({
  marker,
  titleLeft,
  titleRight,
  caption,
}: {
  marker: string;
  titleLeft: string;
  titleRight: string;
  caption: string;
}) {
  return (
    <div className="mb-10 flex flex-wrap items-end justify-between gap-6 border-b border-ink pb-6">
      <div>
        <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-ink-soft">
          {marker}
        </span>
        <h2
          className="mt-2 font-display leading-[0.95] tracking-tight text-ink"
          style={{ fontSize: "clamp(36px, 5.5vw, 72px)" }}
        >
          {titleLeft} <span className="italic">{titleRight}</span>
        </h2>
      </div>
      <p className="max-w-md font-sans text-sm leading-relaxed text-ink-soft md:text-base">
        {caption}
      </p>
    </div>
  );
}
