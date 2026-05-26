"use client";

import { Children, isValidElement, useEffect, useRef, useState, type ReactNode } from "react";

type CopyCodeBlockProps = {
  children: ReactNode;
  className?: string;
};

export function extractCopyText(node: ReactNode): string {
  if (typeof node === "string" || typeof node === "number") {
    return String(node);
  }

  if (Array.isArray(node)) {
    return node.map(extractCopyText).join("");
  }

  if (isValidElement<{ children?: ReactNode }>(node)) {
    return extractCopyText(node.props.children);
  }

  return "";
}

function getCodeLabel(children: ReactNode) {
  const firstChild = Children.toArray(children)[0];
  if (!isValidElement<{ className?: string }>(firstChild)) {
    return "Code";
  }

  const language = firstChild.props.className?.match(/language-([a-z0-9-]+)/i)?.[1];
  if (!language) return "Code";

  const labels: Record<string, string> = {
    bash: "Terminal",
    sh: "Terminal",
    shell: "Terminal",
    http: "HTTP",
    json: "JSON",
    ts: "TypeScript",
    tsx: "TSX",
  };

  return labels[language] ?? language.toUpperCase();
}

export function CopyCodeBlock({ children, className }: CopyCodeBlockProps) {
  const value = extractCopyText(children).trimEnd();
  const label = getCodeLabel(children);

  return (
    <div className="mt-5 overflow-hidden border border-ink bg-ink">
      <div className="flex items-center justify-between gap-3 border-b border-ink bg-paper-deep px-3 py-2">
        <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-ink-soft">
          {label}
        </span>
        <CodeCopyButton value={value} label={label} />
      </div>
      <pre
        className={[
          "m-0 max-w-full overflow-x-auto bg-ink p-4 font-mono text-xs leading-6 text-paper",
          className,
        ]
          .filter(Boolean)
          .join(" ")}
      >
        {children}
      </pre>
    </div>
  );
}

function CodeCopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    };
  }, []);

  async function onClick() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
      timerRef.current = window.setTimeout(() => setCopied(false), 1600);
    } catch {
      /* Clipboard can be blocked in insecure contexts. */
    }
  }

  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={`Copy ${label.toLowerCase()} block`}
      className="inline-flex min-h-9 min-w-[92px] items-center justify-center border-2 border-ink bg-accent px-4 py-1.5 font-mono text-[11px] font-bold uppercase tracking-[0.14em] text-paper shadow-[3px_3px_0_0_var(--ink)] transition-transform hover:-translate-y-0.5 hover:shadow-[4px_4px_0_0_var(--ink)] active:translate-x-[2px] active:translate-y-[2px] active:shadow-none"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}
