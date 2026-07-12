"use client";

import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "md" | "sm";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: Size;
  children: ReactNode;
};

const BASE =
  "group inline-flex items-center justify-center gap-2 select-none whitespace-nowrap font-sans font-medium uppercase tracking-[0.08em] transition-all duration-150 disabled:opacity-40 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";

const VARIANTS: Record<Variant, string> = {
  primary:
    "border border-ink bg-ink text-paper shadow-[6px_6px_0_0_var(--accent)] hover:shadow-[8px_8px_0_0_var(--accent)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-[2px_2px_0_0_var(--accent)] disabled:shadow-[6px_6px_0_0_var(--ink-faint)]",
  secondary:
    "border border-ink bg-paper text-ink shadow-[4px_4px_0_0_var(--ink)] hover:bg-ink hover:text-paper hover:shadow-[6px_6px_0_0_var(--accent)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-[1px_1px_0_0_var(--ink)] disabled:shadow-[4px_4px_0_0_var(--ink-faint)]",
  ghost:
    "text-ink underline-offset-[6px] decoration-[1.5px] hover:underline",
  danger:
    "border border-alert bg-alert text-paper shadow-[4px_4px_0_0_var(--ink)] hover:shadow-[6px_6px_0_0_var(--ink)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-[1px_1px_0_0_var(--ink)]",
};

const SIZES: Record<Size, string> = {
  md: "px-6 py-3 text-[13px]",
  sm: "px-3 py-1.5 text-[11px]",
};

export const Button = forwardRef<HTMLButtonElement, Props>(function Button(
  { variant = "primary", size = "md", className = "", children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      className={[BASE, VARIANTS[variant], SIZES[size], className].filter(Boolean).join(" ")}
      {...rest}
    >
      {children}
    </button>
  );
});
