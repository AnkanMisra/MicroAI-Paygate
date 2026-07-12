import type { ComponentPropsWithoutRef } from "react";
import type { MDXComponents } from "mdx/types";
import { CopyCodeBlock } from "@/components/docs/copy-code-block";

function cx(...classes: Array<string | undefined>) {
  return classes.filter(Boolean).join(" ");
}

export function useMDXComponents(components: MDXComponents): MDXComponents {
  return {
    h1: (props: ComponentPropsWithoutRef<"h1">) => (
      <h1
        {...props}
        className={cx(
          "font-display text-[44px] leading-[0.95] tracking-tight text-ink md:text-[64px]",
          props.className,
        )}
      />
    ),
    h2: (props: ComponentPropsWithoutRef<"h2">) => (
      <h2
        {...props}
        className={cx(
          "mt-12 scroll-mt-28 border-t border-ink pt-8 font-display text-[30px] leading-none tracking-tight text-ink md:text-[42px]",
          props.className,
        )}
      />
    ),
    h3: (props: ComponentPropsWithoutRef<"h3">) => (
      <h3
        {...props}
        className={cx(
          "mt-8 font-display text-[23px] leading-tight text-ink md:text-[28px]",
          props.className,
        )}
      />
    ),
    p: (props: ComponentPropsWithoutRef<"p">) => (
      <p
        {...props}
        className={cx("mt-4 max-w-3xl font-sans text-base leading-7 text-ink-soft", props.className)}
      />
    ),
    a: (props: ComponentPropsWithoutRef<"a">) => (
      <a
        {...props}
        className={cx(
          "font-medium text-accent underline decoration-accent/40 underline-offset-4 transition-colors hover:text-accent-press",
          props.className,
        )}
      />
    ),
    ul: (props: ComponentPropsWithoutRef<"ul">) => (
      <ul {...props} className={cx("mt-4 list-disc space-y-2 pl-6 text-ink-soft", props.className)} />
    ),
    ol: (props: ComponentPropsWithoutRef<"ol">) => (
      <ol
        {...props}
        className={cx("mt-4 list-decimal space-y-2 pl-6 text-ink-soft", props.className)}
      />
    ),
    li: (props: ComponentPropsWithoutRef<"li">) => (
      <li {...props} className={cx("pl-1 leading-7", props.className)} />
    ),
    code: (props: ComponentPropsWithoutRef<"code">) => {
      const isBlockCode = props.className?.includes("language-");
      return (
        <code
          {...props}
          className={cx(
            isBlockCode
              ? "font-mono text-xs text-paper"
              : "border border-ink/20 bg-paper-deep px-1.5 py-0.5 font-mono text-[0.86em] text-ink",
            props.className,
          )}
        />
      );
    },
    pre: ({ children, className }: ComponentPropsWithoutRef<"pre">) => (
      <CopyCodeBlock className={className}>{children}</CopyCodeBlock>
    ),
    blockquote: (props: ComponentPropsWithoutRef<"blockquote">) => (
      <blockquote
        {...props}
        className={cx(
          "mt-6 border-l-4 border-accent bg-accent-soft px-5 py-4 font-sans text-sm leading-7 text-ink",
          props.className,
        )}
      />
    ),
    table: (props: ComponentPropsWithoutRef<"table">) => (
      <div className="mt-6 overflow-x-auto border border-ink">
        <table {...props} className={cx("min-w-full border-collapse text-left", props.className)} />
      </div>
    ),
    thead: (props: ComponentPropsWithoutRef<"thead">) => (
      <thead {...props} className={cx("bg-ink text-paper", props.className)} />
    ),
    tbody: (props: ComponentPropsWithoutRef<"tbody">) => (
      <tbody {...props} className={cx("divide-y divide-ink", props.className)} />
    ),
    tr: (props: ComponentPropsWithoutRef<"tr">) => <tr {...props} />,
    th: (props: ComponentPropsWithoutRef<"th">) => (
      <th
        {...props}
        className={cx(
          "px-4 py-3 font-mono text-[10px] uppercase tracking-[0.14em]",
          props.className,
        )}
      />
    ),
    td: (props: ComponentPropsWithoutRef<"td">) => (
      <td {...props} className={cx("px-4 py-3 align-top text-sm leading-6 text-ink-soft", props.className)} />
    ),
    ...components,
  };
}
