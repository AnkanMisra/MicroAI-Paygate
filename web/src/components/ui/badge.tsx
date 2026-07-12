import type { ReactNode } from "react";

type Tone = "default" | "accent" | "ok" | "alert" | "muted" | "ink";

type Props = {
  tone?: Tone;
  children: ReactNode;
  className?: string;
};

const TONES: Record<Tone, string> = {
  default: "border-ink bg-paper text-ink",
  accent: "border-accent bg-accent-soft text-accent",
  ok: "border-ok bg-ok-soft text-ok",
  alert: "border-alert bg-alert-soft text-alert",
  muted: "border-ink-faint bg-paper text-ink-soft",
  ink: "border-ink bg-ink text-paper",
};

export function Badge({ tone = "default", children, className = "" }: Props) {
  return (
    <span
      className={[
        "inline-flex items-center gap-1.5 border px-2 py-0.5 font-mono text-[10px] uppercase tracking-[0.12em] tnum",
        TONES[tone],
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {children}
    </span>
  );
}
