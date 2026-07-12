export type DocsNavItem = {
  href: string;
  label: string;
  description: string;
};

export const DOCS_NAV_ITEMS: DocsNavItem[] = [
  {
    href: "/docs",
    label: "Overview",
    description: "What MicroAI Paygate is and how to read the platform docs.",
  },
  {
    href: "/docs/quickstart",
    label: "Quickstart",
    description: "Run the stack locally and complete the first paid request.",
  },
  {
    href: "/docs/sdk",
    label: "SDK",
    description: "Use the private TypeScript SDK from an AI API client.",
  },
  {
    href: "/docs/api",
    label: "API",
    description: "Gateway endpoints, headers, receipts, and OpenAPI links.",
  },
  {
    href: "/docs/protocol",
    label: "Protocol",
    description: "The current custom x402-style challenge, signing, and receipt flow.",
  },
  {
    href: "/docs/architecture",
    label: "Architecture",
    description: "Service boundaries, data flow, and deployment topology.",
  },
  {
    href: "/docs/operations",
    label: "Operations",
    description: "Environment variables, providers, tests, and troubleshooting.",
  },
  {
    href: "/docs/security-limits",
    label: "Security & Limits",
    description: "Security model, current limitations, and roadmap boundaries.",
  },
];
