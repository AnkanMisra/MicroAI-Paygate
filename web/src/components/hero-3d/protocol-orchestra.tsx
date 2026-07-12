"use client";

import { useEffect, useState, useSyncExternalStore } from "react";

// SSR-safe subscription to the OS's prefers-reduced-motion preference.
// useSyncExternalStore avoids the set-state-in-effect anti-pattern flagged
// by react-hooks/set-state-in-effect.
function subscribeReducedMotion(cb: () => void): () => void {
  if (typeof window === "undefined" || !window.matchMedia) return () => {};
  const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
  mq.addEventListener("change", cb);
  return () => mq.removeEventListener("change", cb);
}
function getReducedMotionSnapshot(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}
function getReducedMotionServerSnapshot(): boolean {
  return false;
}

const STEPS = [
  { n: "01", name: "Request", verb: "sending request" },
  { n: "02", name: "Challenge", verb: "issuing 402 challenge" },
  { n: "03", name: "Sign", verb: "signing EIP-712 typed data" },
  { n: "04", name: "Verify", verb: "verifying signature" },
  { n: "05", name: "Generate", verb: "running AI provider" },
  { n: "06", name: "Receipt", verb: "signing receipt" },
] as const;

const CX = 200;
const CY = 200;
const RADIUS = 152;
const NODE_W = 72;
const NODE_H = 44;
const CYCLE_SEC = 6;

const NODES = STEPS.map((s, i) => {
  const angle = -Math.PI / 2 + (i * Math.PI * 2) / 6;
  return {
    ...s,
    angle,
    x: CX + Math.cos(angle) * RADIUS,
    y: CY + Math.sin(angle) * RADIUS,
  };
});

const ARCS = NODES.map((node, i) => {
  const next = NODES[(i + 1) % NODES.length];
  return { from: node, to: next };
});

const PATH_D =
  NODES.map((n, i) => `${i === 0 ? "M" : "L"} ${n.x.toFixed(2)} ${n.y.toFixed(2)}`).join(" ") + " Z";

// Rounded chevron: sharp point in front, soft notch in back.
const CHEVRON_D = "M -9 -7 Q -4 0 -9 7 L 12 0 Z";

export function ProtocolOrchestra3D() {
  const [stepIdx, setStepIdx] = useState(0);
  const [txCount, setTxCount] = useState(42);
  // CSS @media handles class-based keyframes, but SVG SMIL (<animateMotion>,
  // <animate>) is invisible to CSS reduce-motion rules. We read the OS
  // preference and pass repeatCount="1" so the chevron and pulse rings play
  // once then settle. Also freezes the JS-driven caption + TX counter.
  const prefersReduced = useSyncExternalStore(
    subscribeReducedMotion,
    getReducedMotionSnapshot,
    getReducedMotionServerSnapshot,
  );

  // Live caption + tx counter — paused when reduce-motion is on so screens
  // with vestibular triggers don't see content swap every second.
  useEffect(() => {
    if (prefersReduced) return;
    const start = Date.now();
    let lastCycle = 0;

    const id = window.setInterval(() => {
      const t = (Date.now() - start) / 1000;
      const cycleT = t % CYCLE_SEC;
      const idx = Math.min(STEPS.length - 1, Math.floor(cycleT));
      setStepIdx(idx);

      const cycle = Math.floor(t / CYCLE_SEC);
      if (cycle !== lastCycle) {
        lastCycle = cycle;
        setTxCount((c) => c + 1);
      }
    }, 200);
    return () => window.clearInterval(id);
  }, [prefersReduced]);

  const current = STEPS[stepIdx];

  return (
    <div className="relative flex h-full min-h-[380px] w-full flex-col items-center justify-center gap-2 px-3 pb-3 pt-3">
      {/* SVG stage — flat, no 3D tilt or mouse hover */}
      <div className="flex-1 flex items-center justify-center w-full">
        <div className="flat-stage">
          <svg
            viewBox="0 0 400 400"
            className="block w-full max-w-[520px]"
            aria-label="x402-style protocol orchestra"
          >
            <defs>
              <path id="hex-path" d={PATH_D} fill="none" stroke="none" />
              <filter id="cobalt-bloom" x="-50%" y="-50%" width="200%" height="200%">
                <feGaussianBlur stdDeviation="2" result="blur" />
                <feMerge>
                  <feMergeNode in="blur" />
                  <feMergeNode in="SourceGraphic" />
                </feMerge>
              </filter>
            </defs>

            {/* Faint hex skeleton */}
            <path
              d={PATH_D}
              fill="none"
              stroke="var(--ink)"
              strokeWidth="1"
              strokeLinecap="square"
              opacity="0.18"
            />

            {/* Progressive arc fill — each arc lights up solid cobalt as the chevron passes,
                holds for the middle of the cycle, then erases in arrow direction just before the
                chevron returns. The cumulative fill itself IS the trail. */}
            {ARCS.map((a, i) => (
              <line
                key={`fill-${i}`}
                x1={a.from.x}
                y1={a.from.y}
                x2={a.to.x}
                y2={a.to.y}
                stroke="var(--accent)"
                strokeWidth="3"
                strokeLinecap="round"
                pathLength={1}
                strokeDasharray="1 1"
                className="arc-fill"
                style={{ animationDelay: `${i}s` }}
              />
            ))}

            {/* Gateway spokes */}
            {NODES.map((n, i) => (
              <line
                key={`spoke-${n.n}`}
                x1={n.x}
                y1={n.y}
                x2={CX}
                y2={CY}
                stroke="var(--accent)"
                strokeWidth="1.5"
                strokeLinecap="round"
                pathLength={1}
                strokeDasharray="1 1"
                className="spoke"
                style={{ animationDelay: `${i}s` }}
              />
            ))}

            {/* Cobalt pulse rings emanating from gateway — 2 rings, 3s offset.
                Omitted entirely under reduce-motion (motionRepeat=1 still
                animates for a full 6s on first paint, which is too long). */}
            {!prefersReduced && (
              <g pointerEvents="none">
                {[0, 3].map((delay) => (
                  <circle
                    key={delay}
                    cx={CX}
                    cy={CY}
                    r="38"
                    fill="none"
                    stroke="var(--accent)"
                    strokeWidth="1.8"
                  >
                    <animate
                      attributeName="r"
                      from="38"
                      to="118"
                      dur="6s"
                      begin={`${delay}s`}
                      repeatCount="indefinite"
                    />
                    <animate
                      attributeName="opacity"
                      from="0.55"
                      to="0"
                      dur="6s"
                      begin={`${delay}s`}
                      repeatCount="indefinite"
                    />
                    <animate
                      attributeName="stroke-width"
                      from="1.8"
                      to="0.4"
                      dur="6s"
                      begin={`${delay}s`}
                      repeatCount="indefinite"
                    />
                  </circle>
                ))}
              </g>
            )}

            {/* Hard offset shadow behind center marker */}
            <circle cx={CX + 4} cy={CY + 4} r="38" fill="var(--ink)" />

            {/* Center marker */}
            <g transform={`translate(${CX} ${CY})`}>
              <circle r="38" fill="var(--paper)" stroke="var(--ink)" strokeWidth="1.5" />
              <text
                y="-4"
                fontSize="9"
                fontFamily="var(--font-mono)"
                fill="var(--ink-soft)"
                textAnchor="middle"
                style={{ letterSpacing: "0.16em", textTransform: "uppercase" }}
              >
                gateway
              </text>
              <text
                y="14"
                fontSize="16"
                fontFamily="var(--font-display)"
                fontStyle="italic"
                fill="var(--accent)"
                textAnchor="middle"
              >
                x402
              </text>
            </g>

            {/* Main chevron — rounded back, bloom glow.
                Omitted entirely under reduce-motion (without animateMotion
                the chevron would be parked at SVG origin, a stray cobalt
                triangle in the top-left corner). The fully-lit static arc
                fills already convey the protocol's "completed" state. */}
            {!prefersReduced && (
              <path
                d={CHEVRON_D}
                fill="var(--accent)"
                stroke="var(--accent)"
                strokeWidth="1"
                strokeLinejoin="round"
                filter="url(#cobalt-bloom)"
              >
                <animateMotion
                  dur={`${CYCLE_SEC}s`}
                  repeatCount="indefinite"
                  rotate="auto"
                >
                  <mpath href="#hex-path" />
                </animateMotion>
              </path>
            )}

            {/* Node tiles */}
            {NODES.map((n, i) => (
              <g key={n.n} transform={`translate(${n.x} ${n.y})`}>
                <rect
                  x={-NODE_W / 2 + 4}
                  y={-NODE_H / 2 + 4}
                  width={NODE_W}
                  height={NODE_H}
                  fill="var(--ink)"
                />
                <rect
                  x={-NODE_W / 2}
                  y={-NODE_H / 2}
                  width={NODE_W}
                  height={NODE_H}
                  fill="var(--paper)"
                  stroke="var(--ink)"
                  strokeWidth="1.5"
                />
                <rect
                  x={-NODE_W / 2}
                  y={-NODE_H / 2}
                  width={NODE_W}
                  height={NODE_H}
                  fill="var(--accent)"
                  className="node-flash"
                  style={{ animationDelay: `${i}s` }}
                />
                <rect
                  x={-NODE_W / 2}
                  y={-NODE_H / 2}
                  width={NODE_W}
                  height={NODE_H}
                  fill="none"
                  stroke="var(--accent)"
                  strokeWidth="2.5"
                  className="node-outline"
                  style={{ animationDelay: `${i}s` }}
                />
                <text
                  x={-NODE_W / 2 + 5}
                  y={-NODE_H / 2 + 11}
                  fontSize="8"
                  fontFamily="var(--font-mono)"
                  fill="var(--ink-soft)"
                  style={{ letterSpacing: "0.16em" }}
                >
                  {n.n}
                </text>
                <text
                  x="0"
                  y={NODE_H / 2 - 14}
                  fontSize="12"
                  fontFamily="var(--font-sans)"
                  fontWeight={500}
                  fill="var(--ink)"
                  textAnchor="middle"
                >
                  {n.name}
                </text>
              </g>
            ))}

          </svg>
        </div>
      </div>

      {/* Live caption — OUTSIDE the SVG, never overlaps a node.
          aria-hidden because the strip auto-advances every second; assistive
          tech announcing it on a loop would be hostile. The sr-only span
          above gives one static description of the diagram. */}
      <span className="sr-only">
        Live x402-style protocol diagram showing six steps — request, challenge, sign,
        verify, generate, receipt — cycling continuously.
      </span>
      <div className="flex w-full items-baseline justify-center gap-3" aria-hidden>
        <span className="font-mono text-[10px] uppercase tracking-[0.22em] text-ink-soft">
          now ·
        </span>
        <span
          key={current.n}
          className="font-display italic text-ink caption-fade text-balance"
          style={{ fontSize: "clamp(15px, 1.7vw, 22px)" }}
        >
          {current.verb}
          <span className="text-accent">.</span>
        </span>
      </div>

      {/* Top-left plate label */}
      <div className="pointer-events-none absolute left-3 top-3 z-20 font-mono text-[10px] uppercase tracking-[0.22em] text-ink-soft">
        Plate § 06
      </div>

      {/* Top-right LIVE TX counter chip */}
      <div className="pointer-events-none absolute right-3 top-3 z-20 flex items-center gap-1.5 border border-ink bg-paper px-2 py-1">
        <span
          aria-hidden
          className="inline-block size-1.5 bg-accent"
          style={
            prefersReduced
              ? undefined
              : { animation: "live-blink 1.6s ease-in-out infinite" }
          }
        />
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] tnum text-ink">
          TX {String(txCount).padStart(4, "0")} · LIVE
        </span>
      </div>

      <style jsx>{`
        .flat-stage {
          width: 100%;
          display: flex;
          justify-content: center;
          align-items: center;
        }

        /* Subtle background fill that builds up over the cycle */
        :global(.arc-fill) {
          animation: arc-fill-cycle ${CYCLE_SEC}s linear infinite both;
        }
        @keyframes arc-fill-cycle {
          0%   { stroke-dashoffset: 1; }
          17%  { stroke-dashoffset: 0; }
          83%  { stroke-dashoffset: 0; }
          100% { stroke-dashoffset: -1; }
        }

        /* Gateway spokes */
        :global(.spoke) {
          animation: spoke-pulse ${CYCLE_SEC}s linear infinite both;
          opacity: 0;
        }
        @keyframes spoke-pulse {
          0%   { stroke-dashoffset: 1;  opacity: 0; }
          0.5% { opacity: 0.95; }
          10%  { stroke-dashoffset: 0;  opacity: 0.95; }
          16%  { stroke-dashoffset: 0;  opacity: 0.5; }
          25%  { stroke-dashoffset: -1; opacity: 0; }
          100% { stroke-dashoffset: -1; opacity: 0; }
        }

        :global(.node-flash) {
          opacity: 0;
          pointer-events: none;
          animation: flash-pulse ${CYCLE_SEC}s linear infinite both;
        }
        @keyframes flash-pulse {
          0%, 100% { opacity: 0; }
          0.4%    { opacity: 0.55; }
          5%      { opacity: 0.18; }
          11%     { opacity: 0; }
        }

        :global(.node-outline) {
          opacity: 0;
          pointer-events: none;
          animation: outline-pulse ${CYCLE_SEC}s linear infinite both;
        }
        @keyframes outline-pulse {
          0%, 100% { opacity: 0; }
          0.4%    { opacity: 1; }
          7%      { opacity: 0; }
        }

        /* globals.css covers the cross-page keyframes (pulse-dot, stripe-shift,
           reveal-up, copied-pop) but these four are defined locally inside
           styled-jsx and that block isn't reachable from a global @media rule.
           Repeat the reduce-motion guard here so arc-fill / spoke / flash /
           outline don't loop on a vestibular-sensitive OS setting. The SMIL
           chevron and pulse-ring elements above are conditionally rendered
           via the (prefersReduced && ...) JSX guard. */
        @media (prefers-reduced-motion: reduce) {
          :global(.arc-fill),
          :global(.spoke),
          :global(.node-flash),
          :global(.node-outline) {
            animation: none !important;
          }
        }

        .caption-fade {
          animation: caption-in 480ms cubic-bezier(0.2, 0.7, 0.2, 1);
        }
        @keyframes caption-in {
          from { opacity: 0; transform: translateY(4px); }
          to   { opacity: 1; transform: translateY(0); }
        }

        @keyframes live-blink {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.35; }
        }
      `}</style>
    </div>
  );
}
