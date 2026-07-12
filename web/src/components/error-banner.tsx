import type { ClassifiedError } from "@/lib/errors";
import { Button } from "./ui/button";

type Props = {
  error: ClassifiedError;
  onRetry?: () => void;
  onDismiss?: () => void;
};

export function ErrorBanner({ error, onRetry, onDismiss }: Props) {
  return (
    <div role="alert" className="reveal-up relative border border-alert bg-alert-soft p-5 pl-7">
      <div aria-hidden className="absolute inset-y-0 left-0 w-2 bg-alert" />
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 flex-1 space-y-2">
          <h3 className="font-display text-2xl leading-none text-alert">
            {error.title}
          </h3>
          <p className="font-sans text-sm leading-relaxed text-ink">
            {error.message}
          </p>
          {error.detail && (
            <details className="mt-1">
              <summary className="cursor-pointer font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
                show debug detail
              </summary>
              <pre className="mt-2 max-h-32 overflow-auto whitespace-pre-wrap break-all border border-ink-faint bg-paper p-2 font-mono text-[10px] text-ink-soft">
                {error.detail}
              </pre>
            </details>
          )}
        </div>
        <div className="flex flex-shrink-0 items-center gap-2">
          {error.recoverable && onRetry && (
            <Button size="sm" variant="primary" onClick={onRetry}>
              Retry
            </Button>
          )}
          {onDismiss && (
            <Button size="sm" variant="ghost" onClick={onDismiss}>
              Dismiss
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
