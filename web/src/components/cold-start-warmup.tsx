"use client";

import { useEffect, useState } from "react";

const GATEWAY_URL = process.env.NEXT_PUBLIC_GATEWAY_URL;
const VERIFIER_URL = process.env.NEXT_PUBLIC_VERIFIER_URL;

export function createWarmupProbes(
  gatewayUrl: string,
  verifierUrl?: string,
  signal?: AbortSignal,
) {
  const probes: Promise<unknown>[] = [
    fetch(`${gatewayUrl}/healthz`, {
      cache: "no-store",
      signal,
    }).catch(() => {}),
  ];

  if (verifierUrl) {
    probes.push(
      fetch(`${verifierUrl}/health`, {
        cache: "no-store",
        signal,
      }).catch(() => {}),
    );
  }

  return probes;
}

export function ColdStartWarmup() {
  const [warm, setWarm] = useState(!GATEWAY_URL);

  useEffect(() => {
    if (!GATEWAY_URL) return;
    const controller = new AbortController();
    const probes = createWarmupProbes(
      GATEWAY_URL,
      VERIFIER_URL,
      controller.signal,
    );
    Promise.allSettled(probes).then(() => setWarm(true));
    return () => controller.abort();
  }, []);

  if (warm) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="fixed inset-x-0 top-0 z-50 border-b-2 border-ink bg-ink px-4 py-2 text-center font-mono text-[10px] uppercase tracking-[0.18em] tnum text-paper"
    >
      § Free tier wake-up — first request may take ~30 seconds
    </div>
  );
}
