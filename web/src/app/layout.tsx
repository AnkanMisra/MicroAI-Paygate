import type { Metadata } from "next";
import { Geist_Mono } from "next/font/google";
import localFont from "next/font/local";
import "./globals.css";
import { SmoothScroll } from "@/components/smooth-scroll";

// Satoshi Variable (Fontshare, ITF) — humanist grotesk for body / UI.
const satoshi = localFont({
  src: [
    { path: "./fonts/Satoshi-Variable.woff2", weight: "300 900", style: "normal" },
    { path: "./fonts/Satoshi-VariableItalic.woff2", weight: "300 900", style: "italic" },
  ],
  variable: "--font-satoshi",
  display: "swap",
});

// Clash Display Variable (Fontshare, ITF) — neo-brutalist condensed sans
// for all display headlines. Replaces Instrument Serif. The 2026 default
// for brutalist + web3 sites per Awwwards / Fontshare popularity stats.
const clashDisplay = localFont({
  src: [
    { path: "./fonts/ClashDisplay-Variable.woff2", weight: "200 700", style: "normal" },
  ],
  variable: "--font-clash-display",
  display: "swap",
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const DISPLAY_CHAIN_NAME =
  process.env.NEXT_PUBLIC_EXPECTED_CHAIN_NAME ?? "Base Sepolia";

export const metadata: Metadata = {
  title: `MicroAI Paygate — pay-per-call AI, settled on ${DISPLAY_CHAIN_NAME}`,
  description:
    "An x402 payment gateway for AI requests. Sign EIP-712, get a signed receipt, verify the signature client-side.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body
        className={`${satoshi.variable} ${geistMono.variable} ${clashDisplay.variable} bg-paper text-ink antialiased`}
      >
        <SmoothScroll />
        {children}
      </body>
    </html>
  );
}
