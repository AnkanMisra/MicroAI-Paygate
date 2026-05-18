"use client";

import { useEffect, useSyncExternalStore } from "react";
import Lenis from "lenis";

// SSR-safe live read of prefers-reduced-motion. Matches the pattern in
// protocol-orchestra.tsx so toggling Accessibility → Reduce motion while
// the page is open destroys Lenis (and recreates it on toggle-off) instead
// of requiring a refresh.
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

/**
 * SmoothScroll — Apple-style momentum scroll for wheel + trackpad.
 *
 * Lenis's `duration` + `easing` and `lerp` are mutually exclusive. Per the
 * Lenis docs (https://github.com/darkroomengineering/lenis), `lerp` wins
 * when both are set — the `duration`/`easing` pair is bypassed entirely.
 * We use `lerp: 0.1` (the Lenis sweet spot for the "buttery" Apple feel)
 * and explicitly omit `duration`/`easing` so the config doesn't lie about
 * what's actually running.
 *
 * - syncTouch: false keeps mobile iOS/Android on native scroll (their
 *   built-in inertia is already excellent).
 * - prefers-reduced-motion is honored — no Lenis instance is constructed,
 *   browser default scrolling is used. The preference is subscribed at
 *   runtime so toggling it via OS Accessibility tears down the instance
 *   immediately without a refresh.
 */
export function SmoothScroll() {
  const reducedMotion = useSyncExternalStore(
    subscribeReducedMotion,
    getReducedMotionSnapshot,
    getReducedMotionServerSnapshot,
  );

  useEffect(() => {
    if (reducedMotion) return;

    const lenis = new Lenis({
      lerp: 0.1,
      smoothWheel: true,
      syncTouch: false,
    });

    let raf = 0;
    function tick(time: number) {
      lenis.raf(time);
      raf = requestAnimationFrame(tick);
    }
    raf = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(raf);
      lenis.destroy();
    };
  }, [reducedMotion]);

  return null;
}
