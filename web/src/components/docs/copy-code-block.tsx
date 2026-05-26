"use client";

import { Children, isValidElement, type ReactNode } from "react";
import { CopyButton } from "@/components/copy-button";

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
      <div className="flex items-center justify-between gap-3 border-b border-paper/20 bg-ink px-3 py-2">
        <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-paper/70">
          {label}
        </span>
        <CopyButton
          value={value}
          label="Copy"
          copiedLabel="Copied"
          ariaLabel={`Copy ${label.toLowerCase()} block`}
          className="border-paper/40 bg-ink text-paper hover:bg-paper hover:text-ink"
        />
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
