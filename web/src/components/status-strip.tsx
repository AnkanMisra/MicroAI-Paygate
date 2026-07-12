import { STEP_LABELS, type X402Step } from "@/lib/types";

type Props = {
  step: X402Step;
  hasError?: boolean;
};

export function StatusStrip({ step, hasError = false }: Props) {
  const activeIdx =
    step === "idle"
      ? -1
      : step === "done"
        ? STEP_LABELS.length
        : STEP_LABELS.findIndex((s) => s.id === step);

  return (
    <div
      role="status"
      aria-live="polite"
      aria-label={
        step === "idle"
          ? "Idle"
          : step === "done"
            ? "Complete"
            : `Step ${activeIdx + 1} of ${STEP_LABELS.length}`
      }
      className="border border-ink bg-paper"
    >
      <div className="flex items-center justify-between border-b border-ink px-3 py-1.5">
        <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
          x402 flow
        </span>
        <span className="font-mono text-[10px] uppercase tracking-[0.16em] tnum text-ink-soft">
          {step === "idle"
            ? "Ready"
            : step === "done"
              ? "Settled"
              : hasError
                ? "Halted"
                : "Live"}
        </span>
      </div>
      <ol className="grid grid-cols-6">
        {STEP_LABELS.map((s, i) => {
          const isDone = i < activeIdx;
          const isActive = i === activeIdx;
          const isPending = i > activeIdx;
          const isFailed = hasError && isActive;

          return (
            <li
              key={s.id}
              className={[
                "relative min-w-0 border-l border-ink px-3 py-3 first:border-l-0 transition-colors duration-200",
                isFailed && "bg-alert-soft text-alert",
                !isFailed && isDone && "bg-ink text-paper",
                !isFailed && isActive && "bg-paper text-ink",
                !isFailed && isPending && "bg-paper-deep text-ink-faint",
              ]
                .filter(Boolean)
                .join(" ")}
            >
              <div className="flex items-center gap-1.5">
                <span className="font-mono text-[10px] tracking-[0.12em] tnum">
                  {String(s.index).padStart(2, "0")}
                </span>
                {isActive && !isFailed && (
                  <span aria-hidden className="size-1.5 bg-accent pulse-dot" />
                )}
                {isDone && !isFailed && (
                  <span aria-hidden className="font-mono text-[10px] leading-none">
                    ✓
                  </span>
                )}
                {isFailed && (
                  <span aria-hidden className="font-mono text-[10px] leading-none">
                    ✗
                  </span>
                )}
              </div>
              <div className="mt-2 truncate font-sans text-[11px] font-medium uppercase tracking-[0.06em]">
                {s.short}
              </div>
              {isActive && !isFailed && (
                <div aria-hidden className="stripe-shift absolute inset-x-0 bottom-0 h-[3px]" />
              )}
              {isDone && !isFailed && (
                <div aria-hidden className="absolute inset-x-0 bottom-0 h-[3px] bg-accent" />
              )}
              {isFailed && (
                <div aria-hidden className="absolute inset-x-0 bottom-0 h-[3px] bg-alert" />
              )}
            </li>
          );
        })}
      </ol>
    </div>
  );
}
