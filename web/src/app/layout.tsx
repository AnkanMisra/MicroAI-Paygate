import type { Metadata } from "next";
import { Geist_Mono, Instrument_Serif } from "next/font/google";
import localFont from "next/font/local";
import "./globals.css";
import { SmoothScroll } from "@/components/smooth-scroll";

// Satoshi Variable (Fontshare, ITF) — humanist grotesk for body / UI.
// Replaces Geist Sans. Two .woff2 files cover the 300-900 weight range
// for both normal and italic styles. Thematic nod: the project's sample
// prompt quotes Satoshi Nakamoto's whitepaper.
const satoshi = localFont({
  src: [
    {
      path: "./fonts/Satoshi-Variable.woff2",
      weight: "300 900",
      style: "normal",
    },
    {
      path: "./fonts/Satoshi-VariableItalic.woff2",
      weight: "300 900",
      style: "italic",
    },
  ],
  variable: "--font-satoshi",
  display: "swap",
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const instrumentSerif = Instrument_Serif({
  variable: "--font-instrument-serif",
  subsets: ["latin"],
  weight: "400",
  style: ["normal", "italic"],
});

export const metadata: Metadata = {
  title: "MicroAI Paygate — pay-per-call AI, settled on Base Sepolia",
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
        className={`${satoshi.variable} ${geistMono.variable} ${instrumentSerif.variable} bg-paper text-ink antialiased`}
      >
        <SmoothScroll />
        {children}
      </body>
    </html>
  );
}
