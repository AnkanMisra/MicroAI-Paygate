"use client";

import { useEffect } from "react";
import Lenis from "lenis";

/**
 * SmoothScroll — Apple-style momentum scroll for wheel + trackpad.
 *
 * Mount once in the root layout. Returns no DOM (`null`); all behavior is in
 * a single `useEffect` that owns the Lenis instance and its RAF loop.
 *
 * Design notes:
 * - duration / easing tuned to match apple.com's settled feel (~1.2s,
 *   easeOutExpo). lerp 0.1 is the Lenis-recommended sweet spot for the
 *   "slightly laggy, very buttery" character.
 * - syncTouch: false keeps mobile iOS/Android on native scroll, which is
 *   already excellent and shouldn't be double-eased.
 * - prefers-reduced-motion is honored with a full opt-out — no Lenis
 *   instance is constructed, browser default scrolling is used.
 * - Sticky elements (the nav), anchor links (#try, #protocol), and CSS
 *   keyframe animations (ticker, orchestra) are unaffected by Lenis — that's
 *   the library's core design promise.
 */
export function SmoothScroll() {
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    const lenis = new Lenis({
      duration: 1.2,
      easing: (t) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
      smoothWheel: true,
      syncTouch: false,
      lerp: 0.1,
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
  }, []);

  return null;
}
