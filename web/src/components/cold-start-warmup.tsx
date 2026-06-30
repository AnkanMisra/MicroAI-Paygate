"use client";

import { useEffect, useState } from "react";

function getGatewayUrl() {
  return process.env.NEXT_PUBLIC_GATEWAY_URL;
}

function getVerifierUrl() {
  return process.env.NEXT_PUBLIC_VERIFIER_URL;
}

function createWarmupProbes(
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
  const [warm, setWarm] = useState(!getGatewayUrl());

  useEffect(() => {
    const gatewayUrl = getGatewayUrl();
    const verifierUrl = getVerifierUrl();

    if (!gatewayUrl) return;

    const controller = new AbortController();

    const probes = createWarmupProbes(
      gatewayUrl,
      verifierUrl,
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